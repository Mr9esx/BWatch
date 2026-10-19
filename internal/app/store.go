package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"
)

type CreatorInput struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	Interval      int    `json:"interval"`
	Quality       string `json:"quality"`
	History       bool   `json:"history"`
	WatchVideos   *bool  `json:"watchVideos,omitempty"`
	WatchDynamics bool   `json:"watchDynamics"`
}
type Creator struct {
	ID                     string          `json:"id"`
	Name                   string          `json:"name"`
	Avatar                 string          `json:"avatar,omitempty"`
	URL                    string          `json:"url"`
	Platform               string          `json:"platform"`
	Interval               int             `json:"interval"`
	Quality                string          `json:"quality"`
	History                bool            `json:"history"`
	WatchVideos            bool            `json:"watchVideos"`
	WatchDynamics          bool            `json:"watchDynamics"`
	Enabled                bool            `json:"enabled"`
	CreatedAt              time.Time       `json:"createdAt"`
	LastChecked            *time.Time      `json:"lastChecked"`
	LastError              string          `json:"lastError"`
	FailureCount           int             `json:"failureCount"`
	NextCheckAt            *time.Time      `json:"nextCheckAt"`
	Initialized            bool            `json:"initialized"`
	DynamicInitialized     bool            `json:"dynamicInitialized"`
	Seen                   map[string]bool `json:"seen"`
	SeenDynamics           map[string]bool `json:"seenDynamics"`
	HistoryJob             *HistoryJob     `json:"historyJob,omitempty"`
	ContentCatalog         map[string]bool `json:"contentCatalog,omitempty"`
	ContentCatalogComplete bool            `json:"contentCatalogComplete"`
	ContentTotals          *ContentTotals  `json:"contentTotals,omitempty"`
}
type Entry struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	Thumbnail string   `json:"thumbnail"`
	Duration  float64  `json:"duration"`
	Content   string   `json:"content,omitempty"`
	Images    []string `json:"images,omitempty"`
}
type Task struct {
	ID           string     `json:"id"`
	CreatorID    string     `json:"creatorId"`
	CreatorName  string     `json:"creatorName"`
	VideoID      string     `json:"videoId"`
	Kind         string     `json:"kind"`
	Title        string     `json:"title"`
	URL          string     `json:"url"`
	Thumbnail    string     `json:"thumbnail"`
	Duration     float64    `json:"duration"`
	Quality      string     `json:"quality"`
	Status       string     `json:"status"`
	Progress     float64    `json:"progress"`
	Speed        string     `json:"speed"`
	ETA          string     `json:"eta"`
	Error        string     `json:"error"`
	File         string     `json:"file"`
	Files        []string   `json:"files,omitempty"`
	Content      string     `json:"content,omitempty"`
	Images       []string   `json:"images,omitempty"`
	Bytes        int64      `json:"bytes"`
	CreatedAt    time.Time  `json:"createdAt"`
	FinishedAt   *time.Time `json:"finishedAt"`
	HistoryJobID string     `json:"historyJobId,omitempty"`
}
type Settings struct {
	Concurrency         int     `json:"concurrency"`
	Interval            int     `json:"interval"`
	Quality             string  `json:"quality"`
	CookiesFile         string  `json:"cookiesFile"`
	ProxyURL            string  `json:"proxyURL"`
	MaxDownloadSpeed    float64 `json:"maxDownloadSpeed"`
	DownloadSubtitles   bool    `json:"downloadSubtitles"`
	SaveThumbnail       bool    `json:"saveThumbnail"`
	BilibiliMinInterval int     `json:"bilibiliMinInterval"`
	BilibiliMaxInterval int     `json:"bilibiliMaxInterval"`
}
type Event struct {
	ID      string    `json:"id"`
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}
type State struct {
	Creators               []Creator  `json:"creators"`
	Tasks                  []Task     `json:"tasks"`
	Events                 []Event    `json:"events"`
	Settings               Settings   `json:"settings"`
	Paused                 bool       `json:"paused"`
	NextBilibiliDownloadAt *time.Time `json:"nextBilibiliDownloadAt,omitempty"`
	NextHistoryPageAt      *time.Time `json:"nextHistoryPageAt,omitempty"`
	BilibiliBlockedUntil   *time.Time `json:"bilibiliBlockedUntil,omitempty"`
	BilibiliRiskCount      int        `json:"bilibiliRiskCount,omitempty"`
}
type App struct {
	mu          sync.Mutex
	state       State
	statePath   string
	downloadDir string
	binary      string
	running     map[string]context.CancelFunc
	checking    map[string]bool
	httpClient  *http.Client
	guestMu     sync.Mutex
	loginMu     sync.Mutex
	login       *loginSession
	accountMu   sync.Mutex
	account     accountCache
	historyBusy bool
	metrics     resourceSampler
}

// New opens a single-process store. A failed transaction never changes memory.
func New(dataDir, downloadDir, binary string) (*App, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(downloadDir)
	if err != nil {
		return nil, err
	}
	a := &App{statePath: filepath.Join(dataDir, "state.json"), downloadDir: absolute, binary: binary, running: map[string]context.CancelFunc{}, checking: map[string]bool{}, httpClient: &http.Client{Timeout: 30 * time.Second, CheckRedirect: checkRedirect}}
	a.state = State{Creators: []Creator{}, Tasks: []Task{}, Events: []Event{}, Settings: Settings{Concurrency: 1, Interval: 10, Quality: "best", SaveThumbnail: true, BilibiliMinInterval: 1, BilibiliMaxInterval: 3}}
	data, err := os.ReadFile(a.statePath)
	if err == nil {
		if err = json.Unmarshal(data, &a.state); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for i := range a.state.Tasks {
		if a.state.Tasks[i].Status == "downloading" {
			a.state.Tasks[i].Status = "queued"
		}
	}
	for i := range a.state.Creators {
		c := &a.state.Creators[i]
		if c.Seen == nil {
			c.Seen = map[string]bool{}
		}
		if c.SeenDynamics == nil {
			c.SeenDynamics = map[string]bool{}
		}
	}
	if err := a.persist(); err != nil {
		return nil, err
	}
	return a, nil
}
func cloneState(s State) State {
	data, _ := json.Marshal(s)
	var out State
	_ = json.Unmarshal(data, &out)
	return out
}
func (a *App) Snapshot() State { a.mu.Lock(); defer a.mu.Unlock(); return cloneState(a.state) }
func (a *App) persist() error {
	data, err := json.MarshalIndent(a.state, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(a.statePath), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temp.Name(), a.statePath)
}
func (a *App) transaction(fn func() error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	before := cloneState(a.state)
	if err := fn(); err != nil {
		a.state = before
		return err
	}
	if reflect.DeepEqual(a.state, before) {
		return nil
	}
	if err := a.persist(); err != nil {
		a.state = before
		return err
	}
	return nil
}
