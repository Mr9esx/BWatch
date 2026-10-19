package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ContentTotals struct {
	Videos    *int      `json:"videos,omitempty"`
	Dynamics  *int      `json:"dynamics,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

// 独立补齐/刷新统计，不受内容检查间隔影响；复用检查名额避免同一 UP 并发请求。
func (a *App) startContentTotalsRefresh(ctx context.Context) bool {
	a.mu.Lock()
	now := time.Now()
	if ctx.Err() != nil || a.state.Paused || future(a.state.BilibiliBlockedUntil, now) || len(a.checking) >= 2 {
		a.mu.Unlock()
		return false
	}
	for _, c := range a.state.Creators {
		if !c.Enabled || c.Platform != "bilibili" || a.checking[c.ID] || (c.ContentTotals != nil && now.Sub(c.ContentTotals.CheckedAt) < time.Hour) {
			continue
		}
		a.checking[c.ID] = true
		settings := a.state.Settings
		a.mu.Unlock()
		go func() {
			defer func() { a.mu.Lock(); delete(a.checking, c.ID); a.mu.Unlock() }()
			if err := a.refreshContentTotals(ctx, c, settings); err != nil && ctx.Err() == nil {
				log.Printf("刷新订阅总量失败: %v", err)
			}
		}()
		return true
	}
	a.mu.Unlock()
	return false
}

func (a *App) fetchContentTotals(ctx context.Context, c Creator, s Settings) (*ContentTotals, error) {
	cookiePath, err := a.bilibiliCookies(ctx, s)
	if err != nil {
		return nil, err
	}
	cookie, err := cookiesForBilibili(cookiePath)
	if err != nil {
		return nil, err
	}
	client, err := a.clientFor(s)
	if err != nil {
		return nil, err
	}
	defer client.CloseIdleConnections()
	u, _ := url.Parse(c.URL)
	uid := strings.Split(strings.Trim(u.Path, "/"), "/")[0]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.bilibili.com/x/space/navnum?mid="+uid, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", c.URL)
	req.Header.Set("Cookie", cookie)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("B 站总量接口 HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Code int `json:"code"`
		Data *struct {
			Video *int `json:"video"`
			Opus  *int `json:"opus"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("总量响应解析失败: %w", err)
	}
	if payload.Code != 0 {
		return nil, fmt.Errorf("B 站总量接口错误 %d", payload.Code)
	}
	if payload.Data == nil || (payload.Data.Video == nil && payload.Data.Opus == nil) {
		return nil, errors.New("总量接口未返回 video/opus 字段")
	}
	if (payload.Data.Video != nil && *payload.Data.Video < 0) || (payload.Data.Opus != nil && *payload.Data.Opus < 0) {
		return nil, errors.New("总量接口返回了负数")
	}
	return &ContentTotals{Videos: payload.Data.Video, Dynamics: payload.Data.Opus}, nil
}

// 总量独立于分页目录；按 UP 主缓存一小时，失败也不在每轮监听中反复请求。
func (a *App) refreshContentTotals(ctx context.Context, c Creator, s Settings) error {
	if c.ContentTotals != nil && time.Since(c.ContentTotals.CheckedAt) < time.Hour {
		return nil
	}
	if a.bilibiliBlocked(time.Now()) {
		return nil
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	totals, fetchErr := a.fetchContentTotals(reqCtx, c, s)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return a.transaction(func() error {
		for i := range a.state.Creators {
			current := &a.state.Creators[i]
			if current.ID != c.ID {
				continue
			}
			if current.ContentTotals == nil {
				current.ContentTotals = &ContentTotals{}
			}
			current.ContentTotals.CheckedAt = time.Now()
			if fetchErr != nil {
				if isRiskError(fetchErr.Error()) {
					a.blockBilibiliLocked(fetchErr.Error(), time.Now())
				}
				a.event("info", "「"+current.Name+"」总量暂未刷新: "+fetchErr.Error())
				break
			}
			current.ContentTotals.Videos = totals.Videos
			current.ContentTotals.Dynamics = totals.Dynamics
			break
		}
		return nil
	})
}
