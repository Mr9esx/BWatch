package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

type CreatorInput struct {
	Platform      PlatformID `json:"platform"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	Interval      int        `json:"interval"`
	Quality       string     `json:"quality"`
	History       bool       `json:"history"`
	WatchVideos   *bool      `json:"watchVideos,omitempty"`
	WatchDynamics bool       `json:"watchDynamics"`
}
type Creator struct {
	ID                     string          `json:"id"`
	Name                   string          `json:"name"`
	Avatar                 string          `json:"avatar,omitempty"`
	URL                    string          `json:"url"`
	Platform               PlatformID      `json:"platform"`
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
	ID        string      `json:"id"`
	Kind      ContentKind `json:"kind,omitempty"`
	Title     string      `json:"title"`
	URL       string      `json:"url"`
	Thumbnail string      `json:"thumbnail"`
	Duration  float64     `json:"duration"`
	Content   string      `json:"content,omitempty"`
	Images    []string    `json:"images,omitempty"`
}
type Task struct {
	DetailFetched bool        `json:"detailFetched,omitempty"`
	ID            string      `json:"id"`
	Platform      PlatformID  `json:"platform"`
	CreatorID     string      `json:"creatorId"`
	CreatorName   string      `json:"creatorName"`
	VideoID       string      `json:"videoId"`
	Kind          ContentKind `json:"kind"`
	Title         string      `json:"title"`
	URL           string      `json:"url"`
	Thumbnail     string      `json:"thumbnail"`
	Duration      float64     `json:"duration"`
	Quality       string      `json:"quality"`
	Status        string      `json:"status"`
	Progress      float64     `json:"progress"`
	Speed         string      `json:"speed"`
	ETA           string      `json:"eta"`
	Error         string      `json:"error"`
	File          string      `json:"file"`
	Files         []string    `json:"files,omitempty"`
	Content       string      `json:"content,omitempty"`
	Images        []string    `json:"images,omitempty"`
	Bytes         int64       `json:"bytes"`
	CreatedAt     time.Time   `json:"createdAt"`
	FinishedAt    *time.Time  `json:"finishedAt"`
	HistoryJobID  string      `json:"historyJobId,omitempty"`
}
type Settings struct {
	Concurrency         int     `json:"concurrency"`
	Interval            int     `json:"interval"`
	Quality             string  `json:"quality"`
	BilibiliCookiesFile string  `json:"bilibiliCookiesFile,omitempty"`
	CookiesFile         string  `json:"cookiesFile,omitempty"`
	ProxyURL            string  `json:"proxyURL"`
	MaxDownloadSpeed    float64 `json:"maxDownloadSpeed"`
	DownloadSubtitles   bool    `json:"downloadSubtitles"`
	SaveThumbnail       bool    `json:"saveThumbnail"`
	DownloadMinInterval int     `json:"downloadMinInterval"`
	DownloadMaxInterval int     `json:"downloadMaxInterval"`
	// Legacy fields are retained for state-file migration and older clients.
	BilibiliMinInterval int `json:"bilibiliMinInterval"`
	BilibiliMaxInterval int `json:"bilibiliMaxInterval"`
}
type Event struct {
	ID      string    `json:"id"`
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}
type State struct {
	SchemaVersion      int                      `json:"schemaVersion"`
	Creators           []Creator                `json:"creators"`
	Tasks              []Task                   `json:"tasks"`
	Events             []Event                  `json:"events"`
	Settings           Settings                 `json:"settings"`
	Credentials        map[PlatformID]string    `json:"credentials,omitempty"`
	Paused             bool                     `json:"paused"`
	PlatformCooldowns  map[PlatformID]time.Time `json:"platformCooldowns,omitempty"`
	PlatformRiskCounts map[PlatformID]int       `json:"platformRiskCounts,omitempty"`
	NextDownloadAt     *time.Time               `json:"nextDownloadAt,omitempty"`
	// Legacy timestamp retained so older saved states can be migrated.
	NextBilibiliDownloadAt *time.Time `json:"nextBilibiliDownloadAt,omitempty"`
	NextHistoryPageAt      *time.Time `json:"nextHistoryPageAt,omitempty"`
	BilibiliBlockedUntil   *time.Time `json:"bilibiliBlockedUntil,omitempty"`
	BilibiliRiskCount      int        `json:"bilibiliRiskCount,omitempty"`
}
type App struct {
	mu             sync.Mutex
	state          State
	statePath      string
	downloadDir    string
	binary         string
	running        map[string]context.CancelFunc
	checking       map[string]bool
	historyCacheMu sync.Mutex
	historyCache   map[string]PlatformFeed
	httpClient     *http.Client
	platforms      platformRegistry
	xhsBrowser     *xhsBrowser
	loginMu        sync.Mutex
	login          *loginSession
	xhsAccountMu   sync.Mutex
	xhsAccount     xhsAccountCache
	accountMu      sync.Mutex
	account        accountCache
	historyBusy    bool
	metrics        resourceSampler
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
	a := &App{statePath: filepath.Join(dataDir, "state.json"), downloadDir: absolute, binary: binary, running: map[string]context.CancelFunc{}, checking: map[string]bool{}, historyCache: map[string]PlatformFeed{}, httpClient: &http.Client{Timeout: 30 * time.Second, CheckRedirect: checkRedirect}, platforms: newPlatformRegistry(), xhsBrowser: newXHSBrowser(dataDir)}
	a.state = State{SchemaVersion: 2, Creators: []Creator{}, Tasks: []Task{}, Events: []Event{}, Settings: Settings{Concurrency: 1, Interval: 10, Quality: "best", SaveThumbnail: true, DownloadMinInterval: 1, DownloadMaxInterval: 3, BilibiliMinInterval: 1, BilibiliMaxInterval: 3}}
	data, err := os.ReadFile(a.statePath)
	if err == nil {
		if err = json.Unmarshal(data, &a.state); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	migrateState(&a.state)
	legacyCookiePath := a.state.Settings.BilibiliCookiesFile
	if legacyCookiePath == "" {
		legacyCookiePath = a.state.Settings.CookiesFile
	}
	if legacyCookiePath != "" {
		cookieData, readErr := os.ReadFile(legacyCookiePath)
		if readErr != nil {
			return nil, errors.New("旧版 B 站 Cookie 文件不可读，请重新登录")
		}
		if err := a.saveCredential(PlatformBilibili, cookieData); err != nil {
			return nil, err
		}
		a.state.Settings.BilibiliCookiesFile = ""
		a.state.Settings.CookiesFile = ""
		if absolute, absErr := filepath.Abs(legacyCookiePath); absErr == nil && filepath.Dir(absolute) == filepath.Clean(dataDir) && strings.HasPrefix(filepath.Base(absolute), "bilibili-login-") {
			_ = os.Remove(absolute)
		}
	}
	_ = os.Remove(filepath.Join(dataDir, "xiaohongshu.cookies.txt"))
	if legacyFiles, _ := filepath.Glob(filepath.Join(dataDir, "bilibili-login-*.cookies.txt")); len(legacyFiles) > 0 {
		for _, path := range legacyFiles {
			_ = os.Remove(path)
		}
	}
	for i := range a.state.Tasks {
		if a.state.Tasks[i].Kind == "dynamic" {
			a.state.Tasks[i].Kind = ContentImagePost
		}
		if a.state.Tasks[i].Status == "downloading" {
			a.state.Tasks[i].Status = "queued"
		}
		task := &a.state.Tasks[i]
		if task.Platform == PlatformXiaohongshu && task.Kind == ContentImagePost && task.Status == "completed" && !task.DetailFetched && len(task.Images) == 0 && len(task.Files) == 0 {
			task.Status = "failed"
			task.Progress = 0
			task.FinishedAt = nil
			task.Error = "旧版归档未读取笔记详情，需重新归档以补全图片和正文"
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

func migrateState(state *State) {
	for i := range state.Events {
		state.Events[i].Message = sanitizeCookieFailure(state.Events[i].Message)
	}
	if state.Settings.DownloadMinInterval == 0 {
		state.Settings.DownloadMinInterval = state.Settings.BilibiliMinInterval
		if state.Settings.DownloadMinInterval == 0 {
			state.Settings.DownloadMinInterval = 1
		}
	}
	if state.Settings.DownloadMaxInterval == 0 {
		state.Settings.DownloadMaxInterval = state.Settings.BilibiliMaxInterval
		if state.Settings.DownloadMaxInterval == 0 {
			state.Settings.DownloadMaxInterval = 3
		}
	}
	state.Settings.BilibiliMinInterval = state.Settings.DownloadMinInterval
	state.Settings.BilibiliMaxInterval = state.Settings.DownloadMaxInterval
	if state.NextDownloadAt == nil {
		state.NextDownloadAt = state.NextBilibiliDownloadAt
	}
	if state.BilibiliBlockedUntil != nil {
		if state.PlatformCooldowns == nil {
			state.PlatformCooldowns = map[PlatformID]time.Time{}
		}
		state.PlatformCooldowns[PlatformBilibili] = *state.BilibiliBlockedUntil
	}
	if state.BilibiliRiskCount > 0 {
		if state.PlatformRiskCounts == nil {
			state.PlatformRiskCounts = map[PlatformID]int{}
		}
		state.PlatformRiskCounts[PlatformBilibili] = state.BilibiliRiskCount
	}
	if state.Settings.BilibiliCookiesFile == "" {
		state.Settings.BilibiliCookiesFile = state.Settings.CookiesFile
	}
	state.Settings.CookiesFile = ""
	for i := range state.Tasks {
		task := &state.Tasks[i]
		task.Error = sanitizeCookieFailure(task.Error)
		if task.Platform != "" {
			continue
		}
		for _, creator := range state.Creators {
			if creator.ID == task.CreatorID {
				task.Platform = creator.Platform
				break
			}
		}
		if task.Platform == "" {
			if strings.Contains(task.URL, "bilibili.com/") {
				task.Platform = PlatformBilibili
			} else if strings.Contains(task.URL, "xiaohongshu.com/") {
				task.Platform = PlatformXiaohongshu
			}
		}
	}
	if state.SchemaVersion >= 2 {
		return
	}
	for i := range state.Tasks {
		if state.Tasks[i].Kind == "dynamic" {
			state.Tasks[i].Kind = ContentImagePost
		}
	}
	migrateCatalog := func(catalog *map[string]bool) {
		if *catalog == nil {
			return
		}
		for key, value := range *catalog {
			if strings.HasPrefix(key, "dynamic:") {
				(*catalog)["image_post:"+strings.TrimPrefix(key, "dynamic:")] = value
				delete(*catalog, key)
			}
		}
	}
	for i := range state.Creators {
		creator := &state.Creators[i]
		migrateCatalog(&creator.ContentCatalog)
		if creator.HistoryJob != nil {
			migrateCatalog(&creator.HistoryJob.CatalogEntries)
			migrateCatalog(&creator.HistoryJob.SeenEntries)
		}
	}
	state.SchemaVersion = 2
}
func cloneState(s State) State {
	data, _ := json.Marshal(s)
	var out State
	_ = json.Unmarshal(data, &out)
	return out
}
func (a *App) Snapshot() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := cloneState(a.state)
	out.Credentials = nil
	out.Settings.BilibiliCookiesFile = ""
	out.Settings.CookiesFile = ""
	return out
}
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
