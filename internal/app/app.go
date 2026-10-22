package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"
)

func id() string { var b [8]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func (a *App) event(level, message string) {
	a.state.Events = append(a.state.Events, Event{ID: id(), Time: time.Now(), Level: level, Message: message})
	if len(a.state.Events) > 500 {
		a.state.Events = a.state.Events[len(a.state.Events)-500:]
	}
}

var uidPattern = regexp.MustCompile(`^[0-9]{1,20}$`)

func NormalizeCreatorURL(raw string) (string, error) {
	ref, err := newPlatformRegistry().normalizeCreatorURL(raw)
	if err != nil {
		return "", err
	}
	return ref.URL, nil
}
func validQuality(q string) bool {
	return slices.Contains([]string{"best", "2160", "1080", "720", "480"}, q)
}
func (a *App) AddCreator(in CreatorInput) (Creator, error) {
	return a.AddCreatorContext(context.Background(), in)
}

func (a *App) AddCreatorContext(ctx context.Context, in CreatorInput) (Creator, error) {
	var c Creator
	ref, err := a.platforms.normalizeCreatorForPlatform(in.Platform, in.URL)
	if err != nil {
		return c, err
	}
	if in.Interval < 1 || in.Interval > 1440 || !validQuality(in.Quality) {
		return c, errors.New("检查间隔应为 1–1440 分钟，请选择有效画质")
	}
	platform := ref.Platform
	provider, ok := a.platforms.byPlatform(platform)
	if !ok {
		return c, errors.New("不支持的平台")
	}
	capabilities := provider.Capabilities()
	if capabilities.LoginRequired {
		if a.SavedPlatformAccount(platform).State != "logged_in" {
			return c, fmt.Errorf("请先在平台管理登录%s账号，再添加订阅", contentPlatformLabel(platform))
		}
	}
	if platform == PlatformXiaohongshu && strings.HasPrefix(ref.URL, "xhsid:") {
		redID := strings.TrimPrefix(ref.URL, "xhsid:")
		resolved, resolveErr := a.resolveXHSRedID(ctx, redID)
		if resolveErr != nil {
			return c, resolveErr
		}
		ref = resolved
	}
	if platform == PlatformXiaohongshu && ref.ID == "" {
		ref, err = a.resolveXHSShareURL(ctx, ref.URL)
		if err != nil {
			return c, err
		}
	}
	u := ref.URL
	watch := true
	if in.WatchVideos != nil {
		watch = *in.WatchVideos
	}
	if !watch && !in.WatchDynamics {
		return c, errors.New("至少选择一种监听内容")
	}
	if (watch && !capabilities.Videos) || (in.WatchDynamics && !capabilities.ImagePosts) {
		return c, errors.New("该平台不支持所选内容类型")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = ref.Name
	}
	if name == "" {
		name = ref.ID
		if name == "" {
			name = "小红书博主"
		}
	}
	if len([]rune(name)) > 80 {
		return c, errors.New("名称最多 80 字")
	}
	c = Creator{ID: id(), Name: name, Avatar: ref.Avatar, URL: u, Platform: platform, Interval: in.Interval, Quality: in.Quality, History: in.History, WatchVideos: watch, WatchDynamics: in.WatchDynamics, Enabled: true, CreatedAt: time.Now(), Seen: map[string]bool{}, SeenDynamics: map[string]bool{}}
	err = a.transaction(func() error {
		for _, old := range a.state.Creators {
			sameCreator := old.URL == u
			if old.Platform == platform && ref.ID != "" {
				oldRef, normalizeErr := provider.NormalizeCreatorURL(old.URL)
				sameCreator = sameCreator || (normalizeErr == nil && oldRef.ID == ref.ID)
			}
			if sameCreator {
				return errors.New("已经订阅过这个创作者")
			}
		}
		a.state.Creators = append(a.state.Creators, c)
		a.event("success", "已添加订阅「"+c.Name+"」")
		return nil
	})
	return c, err
}
func (a *App) ApplyScan(creatorID string, entries []Entry, name string) error {
	return a.applyScan(creatorID, entries, name, ContentVideo, "")
}
func (a *App) ApplyDynamicScan(creatorID string, entries []Entry, name string) error {
	return a.applyScan(creatorID, entries, name, ContentImagePost, "")
}
func (a *App) applyScan(creatorID string, entries []Entry, name string, kind ContentKind, avatar string) error {
	return a.transaction(func() error {
		for i := range a.state.Creators {
			c := &a.state.Creators[i]
			if c.ID != creatorID {
				continue
			}
			if !c.Enabled {
				return nil
			}
			if name != "" {
				c.Name = name
			}
			if avatar != "" {
				c.Avatar = avatar
			}
			seen := c.Seen
			initialized := c.Initialized
			if kind == ContentImagePost {
				seen = c.SeenDynamics
				initialized = c.DynamicInitialized
			}
			count := 0
			for n, e := range entries {
				if e.ID == "" || seen[e.ID] {
					continue
				}
				seen[e.ID] = true
				if !initialized && (!c.History || n >= 10) {
					continue
				}
				duplicate := false
				for _, task := range a.state.Tasks {
					if task.CreatorID == c.ID && task.VideoID == e.ID && task.Kind == kind {
						duplicate = true
						break
					}
				}
				if duplicate {
					continue
				}
				title := e.Title
				if title == "" {
					title = map[ContentKind]string{ContentVideo: "视频 ", ContentImagePost: "图文 "}[kind] + e.ID
				}
				a.state.Tasks = append(a.state.Tasks, Task{ID: id(), Platform: c.Platform, CreatorID: c.ID, CreatorName: c.Name, VideoID: e.ID, Kind: kind, Title: title, URL: e.URL, Thumbnail: e.Thumbnail, Duration: e.Duration, Quality: c.Quality, Status: "queued", Content: e.Content, Images: e.Images, CreatedAt: time.Now()})
				count++
			}
			if kind == ContentImagePost {
				c.DynamicInitialized = true
			} else {
				c.Initialized = true
			}
			if count > 0 {
				a.event("info", fmt.Sprintf("「%s」发现 %d 条新内容，已加入下载队列", c.Name, count))
			} else if !initialized {
				a.event("info", "「"+c.Name+"」已建立"+map[ContentKind]string{ContentVideo: "视频", ContentImagePost: "图文"}[kind]+"基线")
			}
			return nil
		}
		return errors.New("订阅不存在")
	})
}
func (a *App) ToggleCreator(creatorID string) error {
	return a.transaction(func() error {
		for i := range a.state.Creators {
			c := &a.state.Creators[i]
			if c.ID == creatorID {
				c.Enabled = !c.Enabled
				a.event("info", "已"+map[bool]string{true: "启用", false: "暂停"}[c.Enabled]+"订阅「"+c.Name+"」")
				return nil
			}
		}
		return errors.New("订阅不存在")
	})
}
func (a *App) DeleteCreator(creatorID string) error {
	return a.transaction(func() error {
		for i, c := range a.state.Creators {
			if c.ID == creatorID {
				a.state.Creators = append(a.state.Creators[:i], a.state.Creators[i+1:]...)
				a.event("info", "已移除订阅「"+c.Name+"」，保留现有下载")
				return nil
			}
		}
		return errors.New("订阅不存在")
	})
}
func (a *App) TaskAction(taskID, action string) error {
	var cancel func()
	err := a.transaction(func() error {
		for i := range a.state.Tasks {
			t := &a.state.Tasks[i]
			if t.ID != taskID {
				continue
			}
			switch action {
			case "pause":
				if t.Status != "queued" && t.Status != "downloading" {
					return errors.New("仅排队或下载中的任务可以暂停")
				}
				t.Status = "paused"
				cancel = a.running[taskID]
			case "resume":
				if t.Status != "paused" {
					return errors.New("仅暂停任务可以继续")
				}
				t.Status = "queued"
				t.Error = ""
			case "retry":
				if t.Status != "failed" {
					return errors.New("仅失败任务可以重试")
				}
				t.Status = "queued"
				t.Error = ""
			case "delete":
				if t.Status == "downloading" {
					cancel = a.running[taskID]
				}
				a.state.Tasks = append(a.state.Tasks[:i], a.state.Tasks[i+1:]...)
			default:
				return errors.New("未知任务操作")
			}
			return nil
		}
		return errors.New("任务不存在")
	})
	if err == nil && cancel != nil {
		cancel()
	}
	return err
}
func (a *App) UpdateSettings(s Settings) error {
	s.BilibiliCookiesFile = ""
	s.CookiesFile = ""
	if s.DownloadMinInterval == 0 {
		s.DownloadMinInterval = s.BilibiliMinInterval
		if s.DownloadMinInterval == 0 {
			s.DownloadMinInterval = 1
		}
	}
	if s.DownloadMaxInterval == 0 {
		s.DownloadMaxInterval = s.BilibiliMaxInterval
		if s.DownloadMaxInterval == 0 {
			s.DownloadMaxInterval = 3
		}
	}
	s.BilibiliMinInterval = s.DownloadMinInterval
	s.BilibiliMaxInterval = s.DownloadMaxInterval
	s.ProxyURL = strings.TrimSpace(s.ProxyURL)
	if err := validateProxy(s.ProxyURL); err != nil {
		return err
	}
	if math.IsNaN(s.MaxDownloadSpeed) || math.IsInf(s.MaxDownloadSpeed, 0) || s.MaxDownloadSpeed < 0 || s.MaxDownloadSpeed > 100000 || (s.MaxDownloadSpeed > 0 && s.MaxDownloadSpeed < 0.001) {
		return errors.New("最大下载速度应为 0 或 0.001–100000 MB/s，0 表示不限速")
	}
	if s.Concurrency < 1 || s.Concurrency > 4 || s.Interval < 1 || s.Interval > 1440 || !validQuality(s.Quality) {
		return errors.New("并发数应为 1–4，检查间隔应为 1–1440 分钟，请选择有效画质")
	}
	if s.DownloadMinInterval < 1 || s.DownloadMaxInterval < s.DownloadMinInterval || s.DownloadMaxInterval > 1440 {
		return errors.New("下载间隔应为 1–1440 分钟，最大值不能小于最小值")
	}
	return a.transaction(func() error { a.state.Settings = s; a.event("success", "全局设置已保存"); return nil })
}
func (a *App) SetPaused(paused bool) error {
	return a.transaction(func() error { a.state.Paused = paused; return nil })
}
