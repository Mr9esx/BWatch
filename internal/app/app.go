package app

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
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
	raw = strings.TrimSpace(raw)
	if uidPattern.MatchString(raw) {
		return "https://space.bilibili.com/" + raw + "/video", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return "", errors.New("请输入 B 站 UID、HTTPS 空间地址或 YouTube 频道地址")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if u.Hostname() == "space.bilibili.com" && len(parts) > 0 && uidPattern.MatchString(parts[0]) {
		return "https://space.bilibili.com/" + parts[0] + "/video", nil
	}
	if u.Hostname() == "www.youtube.com" || u.Hostname() == "youtube.com" {
		if len(parts) > 0 && strings.HasPrefix(parts[0], "@") && len(parts[0]) > 1 {
			return "https://www.youtube.com/" + parts[0] + "/videos", nil
		}
		if len(parts) >= 2 && (parts[0] == "channel" || parts[0] == "c" || parts[0] == "user") && parts[1] != "" {
			return "https://www.youtube.com/" + parts[0] + "/" + parts[1] + "/videos", nil
		}
	}
	return "", errors.New("暂支持 B 站空间和 YouTube 频道，不能使用单个视频地址")
}
func validQuality(q string) bool {
	return slices.Contains([]string{"best", "2160", "1080", "720", "480"}, q)
}
func (a *App) AddCreator(in CreatorInput) (Creator, error) {
	var c Creator
	u, err := NormalizeCreatorURL(in.URL)
	if err != nil {
		return c, err
	}
	if in.Interval < 1 || in.Interval > 1440 || !validQuality(in.Quality) {
		return c, errors.New("检查间隔应为 1–1440 分钟，请选择有效画质")
	}
	platform := "bilibili"
	if strings.Contains(u, "youtube.com") {
		platform = "youtube"
	}
	watch := true
	if in.WatchVideos != nil {
		watch = *in.WatchVideos
	}
	if !watch && !in.WatchDynamics {
		return c, errors.New("至少选择一种监听内容")
	}
	if in.WatchDynamics && platform != "bilibili" {
		return c, errors.New("图文动态目前仅支持 B 站")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		parts := strings.Split(u, "/")
		name = parts[len(parts)-2]
	}
	if len([]rune(name)) > 80 {
		return c, errors.New("名称最多 80 字")
	}
	c = Creator{ID: id(), Name: name, URL: u, Platform: platform, Interval: in.Interval, Quality: in.Quality, History: in.History, WatchVideos: watch, WatchDynamics: in.WatchDynamics, Enabled: true, CreatedAt: time.Now(), Seen: map[string]bool{}, SeenDynamics: map[string]bool{}}
	err = a.transaction(func() error {
		for _, old := range a.state.Creators {
			if old.URL == u {
				return errors.New("已经订阅过这个 UP 主")
			}
		}
		a.state.Creators = append(a.state.Creators, c)
		a.event("success", "已添加订阅「"+c.Name+"」")
		return nil
	})
	return c, err
}
func (a *App) ApplyScan(creatorID string, entries []Entry, name string) error {
	return a.applyScan(creatorID, entries, name, "video", "")
}
func (a *App) ApplyDynamicScan(creatorID string, entries []Entry, name string) error {
	return a.applyScan(creatorID, entries, name, "dynamic", "")
}
func (a *App) applyScan(creatorID string, entries []Entry, name, kind, avatar string) error {
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
			if kind == "dynamic" {
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
					title = map[string]string{"video": "视频 ", "dynamic": "动态 "}[kind] + e.ID
				}
				a.state.Tasks = append(a.state.Tasks, Task{ID: id(), CreatorID: c.ID, CreatorName: c.Name, VideoID: e.ID, Kind: kind, Title: title, URL: e.URL, Thumbnail: e.Thumbnail, Duration: e.Duration, Quality: c.Quality, Status: "queued", Content: e.Content, Images: e.Images, CreatedAt: time.Now()})
				count++
			}
			if kind == "dynamic" {
				c.DynamicInitialized = true
			} else {
				c.Initialized = true
			}
			if count > 0 {
				a.event("info", fmt.Sprintf("「%s」发现 %d 条新内容，已加入下载队列", c.Name, count))
			} else if !initialized {
				a.event("info", "「"+c.Name+"」已建立"+map[string]string{"video": "视频", "dynamic": "动态"}[kind]+"基线")
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
	if s.BilibiliMinInterval < 1 || s.BilibiliMaxInterval < s.BilibiliMinInterval || s.BilibiliMaxInterval > 1440 {
		return errors.New("B 站下载间隔应为 1–1440 分钟，最大值不能小于最小值")
	}
	return a.transaction(func() error { a.state.Settings = s; a.event("success", "全局设置已保存"); return nil })
}
func (a *App) SetPaused(paused bool) error {
	return a.transaction(func() error { a.state.Paused = paused; return nil })
}
