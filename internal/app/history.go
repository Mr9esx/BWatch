package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"strconv"
	"time"
)

type HistoryInput struct {
	Limit         int  `json:"limit"`
	WatchVideos   bool `json:"watchVideos"`
	WatchDynamics bool `json:"watchDynamics"`
}
type HistoryJob struct {
	ID               string          `json:"id"`
	Status           string          `json:"status"`
	Limit            int             `json:"limit"`
	WatchVideos      bool            `json:"watchVideos"`
	WatchDynamics    bool            `json:"watchDynamics"`
	Offset           string          `json:"offset"`
	Pages            int             `json:"pages"`
	Videos           int             `json:"videos"`
	Dynamics         int             `json:"dynamics"`
	NextPageAt       time.Time       `json:"nextPageAt"`
	StartedAt        time.Time       `json:"startedAt"`
	FinishedAt       *time.Time      `json:"finishedAt,omitempty"`
	Error            string          `json:"error"`
	Cursors          map[string]bool `json:"cursors,omitempty"`
	SeenEntries      map[string]bool `json:"seenEntries,omitempty"`
	CatalogEntries   map[string]bool `json:"catalogEntries,omitempty"`
	CatalogFromStart bool            `json:"catalogFromStart,omitempty"`
}

func jitter(min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	return min + time.Duration(rand.Int64N(int64(max-min)+1))
}
func (a *App) StartHistory(creatorID string, in HistoryInput) error {
	if in.Limit < 0 || in.Limit > 100000 || (!in.WatchVideos && !in.WatchDynamics) {
		return errors.New("请选择内容类型，最多新增任务为 0–100000，0 表示全部")
	}
	return a.transaction(func() error {
		for i := range a.state.Creators {
			c := &a.state.Creators[i]
			if c.ID != creatorID {
				continue
			}
			provider, ok := a.platforms.byPlatform(c.Platform)
			if !ok || !provider.Capabilities().History {
				return errors.New("该平台暂不支持历史拉取")
			}
			if !c.Enabled {
				return errors.New("请先启用订阅")
			}
			if c.HistoryJob != nil && (c.HistoryJob.Status == "running" || c.HistoryJob.Status == "paused") {
				return errors.New("已有历史任务，请继续或取消该任务")
			}
			now := time.Now()
			c.HistoryJob = &HistoryJob{ID: id(), Status: "running", Limit: in.Limit, WatchVideos: in.WatchVideos, WatchDynamics: in.WatchDynamics, StartedAt: now, NextPageAt: now}
			a.event("info", "「"+c.Name+"」开始拉取历史内容")
			return nil
		}
		return errors.New("订阅不存在")
	})
}
func (a *App) HistoryAction(creatorID, action string) error {
	return a.transaction(func() error {
		for i := range a.state.Creators {
			c := &a.state.Creators[i]
			if c.ID != creatorID {
				continue
			}
			h := c.HistoryJob
			if h == nil {
				return errors.New("没有历史任务")
			}
			switch action {
			case "pause":
				if h.Status != "running" {
					return errors.New("仅运行中的任务可以暂停")
				}
				h.Status = "paused"
			case "resume":
				if h.Status != "paused" && h.Status != "failed" {
					return errors.New("仅暂停或失败的任务可以继续")
				}
				h.Status = "running"
				h.Error = ""
				h.NextPageAt = time.Now()
			case "cancel":
				if h.Status != "running" && h.Status != "paused" && h.Status != "failed" {
					return errors.New("历史任务已经结束")
				}
				h.Status = "cancelled"
				a.historyCacheMu.Lock()
				delete(a.historyCache, h.ID)
				a.historyCacheMu.Unlock()
			default:
				return errors.New("未知历史操作")
			}
			return nil
		}
		return errors.New("订阅不存在")
	})
}

// One page per call keeps the cursor durable and bounds both memory and queue growth.
func (a *App) ProcessHistoryPage(ctx context.Context, creatorID string) error {
	now := time.Now()
	a.mu.Lock()
	var creator Creator
	for _, c := range a.state.Creators {
		if c.ID == creatorID {
			creator = c
			break
		}
	}
	h := creator.HistoryJob
	pending := 0
	for _, t := range a.state.Tasks {
		if t.Status == "queued" || t.Status == "downloading" || t.Status == "paused" {
			pending++
		}
	}
	if h == nil || h.Status != "running" || !creator.Enabled || a.state.Paused || a.checking[creatorID] || a.historyBusy || pending >= 50 || now.Before(h.NextPageAt) || future(a.state.NextHistoryPageAt, now) || future(a.platformBlockedUntilLocked(creator.Platform), now) {
		a.mu.Unlock()
		return nil
	}
	job := *h
	settings := a.state.Settings
	a.checking[creatorID] = true
	a.historyBusy = true
	a.mu.Unlock()
	defer func() { a.mu.Lock(); delete(a.checking, creatorID); a.historyBusy = false; a.mu.Unlock() }()
	if err := a.transaction(func() error {
		next := now.Add(jitter(15*time.Second, 30*time.Second))
		a.state.NextHistoryPageAt = &next
		return nil
	}); err != nil {
		return err
	}
	timeout := 45 * time.Second
	if creator.Platform == PlatformXiaohongshu {
		timeout = xhsHistoryScanTimeout(settings.XHSAPIRateLimit)
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	feed, err := a.fetchHistoryPage(reqCtx, creator, settings, job)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return a.transaction(func() error {
		for i := range a.state.Creators {
			c := &a.state.Creators[i]
			if c.ID != creatorID || c.HistoryJob == nil || c.HistoryJob.ID != job.ID || c.HistoryJob.Status != "running" {
				continue
			}
			h := c.HistoryJob
			if err != nil {
				h.Error = err.Error()
				if isPlatformRiskError(creator.Platform, h.Error) {
					if creator.Platform == PlatformXiaohongshu {
						a.blockPlatformLocked(creator.Platform, h.Error, time.Now())
					}
					if until := a.platformBlockedUntilLocked(creator.Platform); until != nil {
						h.NextPageAt = *until
					} else {
						h.Status = "failed"
					}
				} else {
					h.Status = "failed"
				}
				a.event("error", "「"+c.Name+"」历史拉取失败: "+h.Error)
				return nil
			}
			if feed.More && (feed.Offset == "" || feed.Offset == job.Offset || h.Cursors[feed.Offset]) {
				h.Status = "failed"
				h.Error = "接口分页游标未前进，已停止拉取"
				return nil
			}
			if feed.Name != "" {
				c.Name = feed.Name
			}
			if feed.Avatar != "" {
				c.Avatar = feed.Avatar
			}
			// 统计整页，两种内容均记录，独立于入队类型、数量上限和背压。
			if job.Offset == "" && h.Pages == 0 {
				h.CatalogFromStart = true
			}
			pageCatalog := contentEntries(feed.Videos, feed.Dynamics)
			mergeCatalog(&h.CatalogEntries, pageCatalog)
			mergeCatalog(&c.ContentCatalog, pageCatalog)
			pending = 0
			if h.SeenEntries == nil {
				h.SeenEntries = map[string]bool{}
			}
			existing := map[string]bool{}
			for _, t := range a.state.Tasks {
				if t.Status == "queued" || t.Status == "downloading" || t.Status == "paused" {
					pending++
				}
				if t.CreatorID == c.ID {
					existing[string(t.Kind)+":"+t.VideoID] = true
				}
			}
			queueFull := false
			add := func(entries []Entry, kind ContentKind) {
				for _, e := range entries {
					a.fillEntryPublicationLocked(c.ID, kind, e)
					if e.ID == "" || (h.Limit > 0 && h.Videos+h.Dynamics >= h.Limit) {
						continue
					}
					key := string(kind) + ":" + e.ID
					if existing[key] || h.SeenEntries[key] {
						h.SeenEntries[key] = true
						continue
					}
					if pending >= 50 {
						queueFull = true
						continue
					}
					title := e.Title
					if title == "" {
						title = e.ID
					}
					a.state.Tasks = append(a.state.Tasks, Task{PublishedAt: e.PublishedAt, ID: id(), Platform: c.Platform, CreatorID: c.ID, CreatorName: c.Name, VideoID: e.ID, Kind: kind, Title: title, URL: e.URL, Thumbnail: e.Thumbnail, Duration: e.Duration, Quality: c.Quality, Status: "queued", Content: e.Content, Images: e.Images, CreatedAt: time.Now(), HistoryJobID: h.ID})
					pending++
					existing[key] = true
					h.SeenEntries[key] = true
					if kind == "video" {
						h.Videos++
						c.Seen[e.ID] = true
					} else {
						h.Dynamics++
						c.SeenDynamics[e.ID] = true
					}
				}
			}
			if h.WatchVideos {
				add(feed.Videos, ContentVideo)
			}
			if h.WatchDynamics {
				add(feed.Dynamics, ContentImagePost)
			}
			h.Error = ""
			h.NextPageAt = *a.state.NextHistoryPageAt
			if queueFull && (h.Limit == 0 || h.Videos+h.Dynamics < h.Limit) {
				return nil
			}
			if h.Cursors == nil {
				h.Cursors = map[string]bool{}
			}
			h.Cursors[job.Offset] = true
			h.Pages++
			h.Offset = feed.Offset
			if !feed.More && h.CatalogFromStart {
				c.ContentCatalog = maps.Clone(h.CatalogEntries)
				c.ContentCatalogComplete = true
			}
			if !feed.More || (h.Limit > 0 && h.Videos+h.Dynamics >= h.Limit) {
				h.Status = "completed"
				at := time.Now()
				h.FinishedAt = &at
				a.event("success", fmt.Sprintf("「%s」历史拉取完成，新增 %d 个下载任务", c.Name, h.Videos+h.Dynamics))
				if creator.Platform == PlatformXiaohongshu {
					a.historyCacheMu.Lock()
					delete(a.historyCache, h.ID)
					a.historyCacheMu.Unlock()
				}
			}
			return nil
		}
		return nil
	})
}

func (a *App) fetchHistoryPage(ctx context.Context, creator Creator, settings Settings, job HistoryJob) (bilibiliFeed, error) {
	if creator.Platform == PlatformBilibili {
		return a.fetchBilibiliPage(ctx, creator, settings, job.Offset)
	}
	if creator.Platform != PlatformXiaohongshu {
		return bilibiliFeed{}, errors.New("该平台暂不支持历史拉取")
	}
	a.historyCacheMu.Lock()
	cached, ok := a.historyCache[job.ID]
	a.historyCacheMu.Unlock()
	if !ok {
		result, err := a.scanXHSAPI(ctx, creator, true)
		if err != nil {
			return bilibiliFeed{}, err
		}
		cached = result
		a.historyCacheMu.Lock()
		a.historyCache[job.ID] = cached
		a.historyCacheMu.Unlock()
	}
	start := 0
	if job.Offset != "" {
		value, err := strconv.Atoi(job.Offset)
		if err != nil || value < 0 {
			return bilibiliFeed{}, errors.New("历史分页位置无效")
		}
		start = value
	}
	if start > len(cached.Entries) {
		return bilibiliFeed{}, errors.New("小红书历史分页位置超过笔记列表")
	}
	end := min(start+30, len(cached.Entries))
	feed := bilibiliFeed{Name: cached.Name, Avatar: cached.Avatar, Complete: end == len(cached.Entries), More: end < len(cached.Entries)}
	for _, entry := range cached.Entries[start:end] {
		if entry.Kind == ContentVideo {
			feed.Videos = append(feed.Videos, entry)
		} else if entry.Kind == ContentImagePost {
			feed.Dynamics = append(feed.Dynamics, entry)
		}
	}
	if feed.More {
		feed.Offset = strconv.Itoa(end)
	}
	return feed, nil
}
func future(t *time.Time, now time.Time) bool { return t != nil && now.Before(*t) }
