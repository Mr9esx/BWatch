package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func publicationTime(value int64) *time.Time {
	if value > 1e12 {
		value /= 1000
	}
	if value < 946684800 || value > 4102444800 {
		return nil
	}
	t := time.Unix(value, 0).UTC()
	return &t
}

func (a *App) localPublication(task Task) *time.Time {
	if task.File == "" {
		return nil
	}
	metadata := filepath.Join(filepath.Dir(task.File), "metadata.json")
	if task.Kind == ContentVideo {
		metadata = strings.TrimSuffix(task.File, filepath.Ext(task.File)) + ".info.json"
	}
	path, err := a.safeFile(metadata)
	if err != nil {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var info struct {
		PublishedAt      *time.Time `json:"publishedAt"`
		Timestamp        float64    `json:"timestamp"`
		ReleaseTimestamp float64    `json:"release_timestamp"`
		PubDate          int64      `json:"pubdate"`
		Time             int64      `json:"time"`
	}
	if json.NewDecoder(io.LimitReader(file, 4<<20)).Decode(&info) != nil {
		return nil
	}
	if info.PublishedAt != nil && publicationTime(info.PublishedAt.Unix()) != nil {
		return info.PublishedAt
	}
	for _, value := range []int64{int64(info.Timestamp), int64(info.ReleaseTimestamp), info.PubDate, info.Time} {
		if at := publicationTime(value); at != nil {
			return at
		}
	}
	return nil
}

func (a *App) fillEntryPublicationLocked(creatorID string, kind ContentKind, entry Entry) {
	if entry.PublishedAt == nil {
		return
	}
	for i := range a.state.Tasks {
		task := &a.state.Tasks[i]
		if task.CreatorID == creatorID && task.Kind == kind && task.VideoID == entry.ID && task.PublishedAt == nil {
			task.PublishedAt = entry.PublishedAt
		}
	}
}

func (a *App) RefreshTaskPublication(ctx context.Context, taskID string) (*time.Time, error) {
	var task *Task
	for _, item := range a.Snapshot().Tasks {
		if item.ID == taskID {
			copy := item
			task = &copy
			break
		}
	}
	if task == nil {
		return nil, errors.New("任务不存在")
	}
	if task.PublishedAt != nil {
		return task.PublishedAt, nil
	}
	at := a.localPublication(*task)
	if at == nil {
		var err error
		at, err = a.fetchSourcePublication(ctx, *task)
		if err != nil {
			return nil, err
		}
	}
	if at == nil {
		return nil, errors.New("平台未提供可确认的发布时间")
	}
	err := a.transaction(func() error {
		for i := range a.state.Tasks {
			if a.state.Tasks[i].ID == taskID {
				a.state.Tasks[i].PublishedAt = at
				return nil
			}
		}
		return errors.New("任务已移除")
	})
	return at, err
}

func (a *App) fetchSourcePublication(ctx context.Context, task Task) (at *time.Time, resultErr error) {
	platform := task.Platform
	if platform == "" && isBilibiliTask(task) {
		platform = PlatformBilibili
	}
	if a.platformBlocked(platform, time.Now()) {
		return nil, errors.New("平台正在冷却，请稍后补齐发布时间")
	}
	defer func() {
		if resultErr != nil && isPlatformRiskError(platform, resultErr.Error()) {
			_ = a.transaction(func() error { a.blockPlatformLocked(platform, resultErr.Error(), time.Now()); return nil })
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if platform == PlatformXiaohongshu {
		u, err := url.Parse(task.URL)
		if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || (u.Hostname() != "www.xiaohongshu.com" && u.Hostname() != "xiaohongshu.com") || u.Path != "/explore/"+task.VideoID {
			return nil, errors.New("小红书笔记地址无效")
		}
		var result struct {
			Time int64 `json:"time"`
		}
		if err := a.callXHSAPI(ctx, xhsAPIRequest{Operation: "note_publication", NoteID: task.VideoID, URL: task.URL}, &result); err != nil {
			return nil, err
		}
		return publicationTime(result.Time), nil
	}
	if platform != PlatformBilibili {
		return nil, errors.New("不支持的平台")
	}
	settings := a.Snapshot().Settings
	cookie, _, err := a.bilibiliCookieHeader(ctx, settings)
	if err != nil {
		return nil, err
	}
	client, err := a.clientFor(settings)
	if err != nil {
		return nil, err
	}
	defer client.CloseIdleConnections()
	endpoint := "https://api.bilibili.com/x/polymer/web-dynamic/v1/detail?id=" + url.QueryEscape(task.VideoID)
	if task.Kind == ContentVideo {
		endpoint = "https://api.bilibili.com/x/web-interface/view?bvid=" + url.QueryEscape(task.VideoID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("B站发布时间接口 HTTP %d", resp.StatusCode)
	}
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			BVID    string      `json:"bvid"`
			PubDate int64       `json:"pubdate"`
			Item    dynamicItem `json:"item"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return nil, errors.New("B站发布时间响应无效")
	}
	if result.Code != 0 {
		return nil, fmt.Errorf("B站发布时间接口错误 %d: %s", result.Code, result.Message)
	}
	if task.Kind == ContentVideo {
		if result.Data.BVID != task.VideoID {
			return nil, errors.New("B站视频详情与任务不一致")
		}
		return publicationTime(result.Data.PubDate), nil
	}
	if result.Data.Item.ID != task.VideoID {
		return nil, errors.New("B站动态详情与任务不一致")
	}
	return publicationTime(result.Data.Item.Modules.Author.PubTS), nil
}
