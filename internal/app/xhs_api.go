package app

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed xhs_api.py
var xhsAPIWorker string

type xhsAPIRequest struct {
	Operation string `json:"operation"`
	NoteID    string `json:"noteID,omitempty"`
	URL       string `json:"url,omitempty"`
	Cookies   string `json:"cookies"`
	Proxy     string `json:"proxy"`
	RedID     string `json:"redID,omitempty"`
	UserID    string `json:"userID,omitempty"`
	Token     string `json:"token,omitempty"`
	Source    string `json:"source,omitempty"`
}

func (a *App) callXHSAPI(ctx context.Context, input xhsAPIRequest, output any) error {
	credential, err := a.loadCredential(PlatformXiaohongshu)
	if err != nil {
		return errors.New("请先在平台管理扫码登录小红书")
	}
	input.Cookies = string(credential)
	input.Proxy = proxyURL(a.Snapshot().Settings)
	if err := validateProxy(input.Proxy); err != nil {
		return err
	}
	runtimeDir := os.Getenv("BILIFETCH_XHS_RUNTIME")
	if runtimeDir == "" {
		runtimeDir = filepath.Join(".tools", "xhs-api")
	}
	runtimeDir, err = filepath.Abs(runtimeDir)
	if err != nil {
		return err
	}
	python := filepath.Join(runtimeDir, "venv", "bin", "python")
	if _, err := os.Stat(python); err != nil {
		return errors.New("小红书 API 运行环境未安装，请运行 sh scripts/install-xhs-api.sh")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, python, "-c", xhsAPIWorker, filepath.Join(runtimeDir, "spider-xhs"))
	configureProcess(command)
	command.Stdin = bytes.NewReader(body)
	result := &cappedBuffer{limit: 20 << 20}
	command.Stdout = result
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("小红书 API 请求未完成: %w", ctx.Err())
		}
		return errors.New("小红书 API 适配器启动失败，请检查 API 运行环境")
	}
	var response struct {
		OK    bool            `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error string          `json:"error"`
	}
	if err := json.Unmarshal(result.Bytes(), &response); err != nil {
		return errors.New("小红书 API 适配器返回无效数据")
	}
	if !response.OK {
		return fmt.Errorf("小红书 API 请求失败: %s", response.Error)
	}
	return json.Unmarshal(response.Data, output)
}

func (a *App) resolveXHSRedID(ctx context.Context, redID string) (CreatorRef, error) {
	var result struct {
		ID     string `json:"id"`
		RedID  string `json:"redID"`
		Name   string `json:"name"`
		Avatar string `json:"avatar"`
		Token  string `json:"token"`
	}
	if err := a.callXHSAPI(ctx, xhsAPIRequest{Operation: "resolve_creator", RedID: redID}, &result); err != nil {
		return CreatorRef{}, err
	}
	if !xhsProfileID.MatchString(result.ID) || !strings.EqualFold(result.RedID, redID) {
		return CreatorRef{}, errors.New("小红书 API 返回的创作者身份与输入账号不匹配")
	}
	u := "https://www.xiaohongshu.com/user/profile/" + result.ID
	if result.Token != "" {
		u += "?" + url.Values{"xsec_token": {result.Token}, "xsec_source": {"pc_search"}}.Encode()
	}
	return CreatorRef{Platform: PlatformXiaohongshu, ID: result.ID, URL: u, Name: result.Name, Avatar: secureXHSImageURL(result.Avatar)}, nil
}

func (a *App) resolveXHSShareURL(ctx context.Context, raw string) (CreatorRef, error) {
	client, err := a.clientFor(a.Snapshot().Settings)
	if err != nil {
		return CreatorRef{}, err
	}
	defer client.CloseIdleConnections()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		host := req.URL.Hostname()
		if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.Port() != "" || req.URL.User != nil || (!isXHSCookieDomain(host) && host != "xhslink.cn" && host != "www.xhslink.cn") {
			return errors.New("小红书分享链接跳转地址无效")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return CreatorRef{}, err
	}
	request.Header.Set("User-Agent", userAgent)
	response, err := client.Do(request)
	if err != nil {
		return CreatorRef{}, fmt.Errorf("解析小红书分享链接失败: %w", err)
	}
	response.Body.Close()
	ref, err := (xiaohongshuAdapter{}).NormalizeCreatorURL(response.Request.URL.String())
	if err != nil || !xhsProfileID.MatchString(ref.ID) {
		return CreatorRef{}, errors.New("分享链接未返回创作者主页，请使用完整主页链接")
	}
	return ref, nil
}

func (a *App) scanXHSAPI(ctx context.Context, creator Creator) (PlatformFeed, error) {
	ref, err := (xiaohongshuAdapter{}).NormalizeCreatorURL(creator.URL)
	if err != nil || ref.ID == "" {
		return PlatformFeed{}, errors.New("小红书创作者主页地址无效")
	}
	u, _ := url.Parse(ref.URL)
	var result struct {
		Notes    []json.RawMessage `json:"notes"`
		Complete bool              `json:"complete"`
	}
	if err := a.callXHSAPI(ctx, xhsAPIRequest{Operation: "scan", UserID: ref.ID, Token: u.Query().Get("xsec_token"), Source: u.Query().Get("xsec_source")}, &result); err != nil {
		return PlatformFeed{}, err
	}
	if !result.Complete {
		return PlatformFeed{}, errors.New("小红书笔记列表读取未完成")
	}
	feed := PlatformFeed{Complete: result.Complete, Entries: []Entry{}}
	seen := map[string]bool{}
	for _, raw := range result.Notes {
		entry, name, avatar, err := parseXHSListNote(raw)
		if err != nil {
			return PlatformFeed{}, err
		}
		if seen[entry.ID] {
			continue
		}
		seen[entry.ID] = true
		feed.Entries = append(feed.Entries, entry)
		if feed.Name == "" {
			feed.Name, feed.Avatar = name, avatar
		}
	}
	return feed, nil
}

// The profile list contains discovery summaries, not complete archive data.
// Resolve each note at download time, when its signed image URLs are fresh.
func (a *App) resolveXHSArchiveTask(ctx context.Context, task Task) (Task, error) {
	u, err := url.Parse(task.URL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || (u.Hostname() != "www.xiaohongshu.com" && u.Hostname() != "xiaohongshu.com") || u.Path != "/explore/"+task.VideoID {
		return task, errors.New("小红书笔记详情地址无效")
	}
	var raw json.RawMessage
	if err := a.callXHSAPI(ctx, xhsAPIRequest{Operation: "note_detail", NoteID: task.VideoID, URL: task.URL}, &raw); err != nil {
		return task, err
	}
	entry, _, _, err := parseXHSListNote(raw)
	if err != nil {
		return task, err
	}
	if entry.ID != task.VideoID || entry.Kind != ContentImagePost {
		return task, errors.New("小红书笔记详情与归档任务不一致")
	}
	task.Title, task.Content, task.Images, task.Thumbnail = entry.Title, entry.Content, entry.Images, entry.Thumbnail
	task.DetailFetched = true
	err = a.transaction(func() error {
		for i := range a.state.Tasks {
			saved := &a.state.Tasks[i]
			if saved.ID != task.ID {
				continue
			}
			if saved.Status != "downloading" {
				return errors.New("归档任务已停止")
			}
			saved.Title, saved.Content, saved.Images, saved.Thumbnail = task.Title, task.Content, task.Images, task.Thumbnail
			saved.DetailFetched = true
			return nil
		}
		return errors.New("归档任务不存在")
	})
	return task, err
}
