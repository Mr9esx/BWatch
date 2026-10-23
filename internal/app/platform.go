package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// PlatformID and ContentKind are the stable identifiers used by the shared
// subscription, task, and archive workflows. Provider-specific protocol code
// stays behind PlatformAdapter so another provider does not need to add
// platform branches to those workflows.
type PlatformID string
type ContentKind string

const (
	PlatformBilibili    PlatformID = "bilibili"
	PlatformXiaohongshu PlatformID = "xiaohongshu"

	ContentVideo     ContentKind = "video"
	ContentImagePost ContentKind = "image_post"
)

type CreatorRef struct {
	Platform PlatformID
	URL      string
	ID       string
	Name     string
	Avatar   string
}

type PlatformAdapter interface {
	ID() PlatformID
	Capabilities() PlatformCapabilities
	NormalizeCreatorURL(raw string) (CreatorRef, error)
	Scan(ctx context.Context, a *App, creator Creator, settings Settings) (PlatformFeed, error)
}

type PlatformCapabilities struct {
	Videos        bool
	ImagePosts    bool
	History       bool
	LoginRequired bool
}

type PlatformFeed struct {
	Entries  []Entry
	Name     string
	Avatar   string
	Complete bool
}

type platformRegistry struct {
	byID []PlatformAdapter
}

func newPlatformRegistry() platformRegistry {
	return platformRegistry{byID: []PlatformAdapter{bilibiliAdapter{}, xiaohongshuAdapter{}}}
}

func (r platformRegistry) byPlatform(id PlatformID) (PlatformAdapter, bool) {
	for _, provider := range r.byID {
		if provider.ID() == id {
			return provider, true
		}
	}
	return nil, false
}

func (r platformRegistry) normalizeCreatorURL(raw string) (CreatorRef, error) {
	for _, provider := range r.byID {
		ref, err := provider.NormalizeCreatorURL(raw)
		if err == nil {
			return ref, nil
		}
	}
	return CreatorRef{}, errors.New("请输入 B 站 UID/空间地址，或小红书 App 分享短链/博主主页链接")
}

func (r platformRegistry) normalizeCreatorForPlatform(platform PlatformID, raw string) (CreatorRef, error) {
	if platform == "" {
		return r.normalizeCreatorURL(raw)
	}
	provider, ok := r.byPlatform(platform)
	if !ok {
		return CreatorRef{}, errors.New("不支持的平台")
	}
	ref, err := provider.NormalizeCreatorURL(raw)
	if err != nil {
		return CreatorRef{}, fmt.Errorf("请输入有效的%s创作者账号或主页链接", contentPlatformLabel(platform))
	}
	return ref, nil
}

type bilibiliAdapter struct{}

func (bilibiliAdapter) ID() PlatformID { return PlatformBilibili }
func (bilibiliAdapter) Capabilities() PlatformCapabilities {
	return PlatformCapabilities{Videos: true, ImagePosts: true, History: true, LoginRequired: true}
}
func (bilibiliAdapter) NormalizeCreatorURL(raw string) (CreatorRef, error) {
	raw = strings.TrimSpace(raw)
	if uidPattern.MatchString(raw) {
		return CreatorRef{Platform: PlatformBilibili, URL: "https://space.bilibili.com/" + raw + "/video", ID: raw}, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Hostname() != "space.bilibili.com" {
		return CreatorRef{}, errors.New("not a Bilibili creator URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 || !uidPattern.MatchString(parts[0]) {
		return CreatorRef{}, errors.New("invalid Bilibili creator URL")
	}
	return CreatorRef{Platform: PlatformBilibili, URL: "https://space.bilibili.com/" + parts[0] + "/video", ID: parts[0]}, nil
}
func (bilibiliAdapter) Scan(ctx context.Context, a *App, creator Creator, settings Settings) (PlatformFeed, error) {
	feed, err := a.scanBilibili(ctx, creator, settings)
	if err != nil {
		return PlatformFeed{}, err
	}
	entries := make([]Entry, 0, len(feed.Videos)+len(feed.Dynamics))
	for _, entry := range feed.Videos {
		entry.Kind = ContentVideo
		entries = append(entries, entry)
	}
	for _, entry := range feed.Dynamics {
		entry.Kind = ContentImagePost
		entries = append(entries, entry)
	}
	return PlatformFeed{Entries: entries, Name: feed.Name, Avatar: feed.Avatar, Complete: feed.Complete}, nil
}

var xhsProfileID = regexp.MustCompile(`^[0-9a-fA-F]{24}$`)
var xhsRedID = regexp.MustCompile(`^[A-Za-z0-9_-]{2,30}$`)

type xiaohongshuAdapter struct{}

func (xiaohongshuAdapter) ID() PlatformID { return PlatformXiaohongshu }
func (xiaohongshuAdapter) Capabilities() PlatformCapabilities {
	return PlatformCapabilities{Videos: true, ImagePosts: true, History: true, LoginRequired: true}
}
func (xiaohongshuAdapter) NormalizeCreatorURL(raw string) (CreatorRef, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "小红书号：") || strings.HasPrefix(raw, "小红书号:") {
		raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(raw, "小红书号："), "小红书号:"))
	}
	if xhsRedID.MatchString(raw) {
		return CreatorRef{Platform: PlatformXiaohongshu, URL: "xhsid:" + raw, ID: raw}, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return CreatorRef{}, errors.New("not an Xiaohongshu creator URL")
	}
	host := strings.ToLower(u.Hostname())
	if host == "xhslink.cn" || host == "www.xhslink.cn" {
		if !strings.HasPrefix(u.Path, "/o/") || len(strings.Trim(strings.TrimPrefix(u.Path, "/o/"), "/")) < 4 {
			return CreatorRef{}, errors.New("invalid Xiaohongshu share URL")
		}
		return CreatorRef{Platform: PlatformXiaohongshu, URL: u.String()}, nil
	}
	if host != "xiaohongshu.com" && host != "www.xiaohongshu.com" {
		return CreatorRef{}, errors.New("not an Xiaohongshu creator URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "user" || parts[1] != "profile" || !xhsProfileID.MatchString(parts[2]) {
		return CreatorRef{}, errors.New("请输入小红书博主主页，不能使用笔记链接")
	}
	u.Host = "www.xiaohongshu.com"
	u.Path = "/user/profile/" + parts[2]
	u.Fragment = ""
	query := url.Values{}
	if token := u.Query().Get("xsec_token"); token != "" {
		query.Set("xsec_token", token)
	}
	if source := u.Query().Get("xsec_source"); source != "" {
		query.Set("xsec_source", source)
	}
	u.RawQuery = query.Encode()
	return CreatorRef{Platform: PlatformXiaohongshu, URL: u.String(), ID: strings.ToLower(parts[2])}, nil
}
func (xiaohongshuAdapter) Scan(ctx context.Context, a *App, creator Creator, _ Settings) (PlatformFeed, error) {
	return a.scanXHSAPI(ctx, creator, false)
}

func (p PlatformID) String() string { return string(p) }

func contentKindLabel(kind ContentKind) string {
	switch kind {
	case ContentVideo:
		return "视频"
	case ContentImagePost:
		return "图文"
	default:
		return fmt.Sprintf("内容(%s)", kind)
	}
}
