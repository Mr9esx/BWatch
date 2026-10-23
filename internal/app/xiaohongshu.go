package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

// xhsBrowser owns the persistent, visible browser profile used for XHS login
// and page-originated requests. Keeping the page in control of its own API
// calls also keeps short-lived signatures and challenge state out of Go logs.
type xhsBrowser struct {
	mu          sync.Mutex
	profileDir  string
	allocCancel context.CancelFunc
	browser     context.Context
	browserStop context.CancelFunc
	page        context.Context
	pageStop    context.CancelFunc
}

func newXHSBrowser(dataDir string) *xhsBrowser {
	return &xhsBrowser{profileDir: filepath.Join(dataDir, "xiaohongshu-browser")}
}

func (b *xhsBrowser) start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.browser != nil && b.browser.Err() == nil {
		probe, cancel := context.WithTimeout(b.browser, 2*time.Second)
		_, _, _, _, _, err := cdpbrowser.GetVersion().Do(cdp.WithExecutor(probe, chromedp.FromContext(b.browser).Browser))
		cancel()
		if err == nil {
			return nil
		}
	}
	if b.pageStop != nil {
		b.pageStop()
	}
	if b.browserStop != nil {
		b.browserStop()
	}
	if b.allocCancel != nil {
		b.allocCancel()
	}
	b.page, b.browser = nil, nil
	b.pageStop, b.browserStop, b.allocCancel = nil, nil, nil
	if err := os.MkdirAll(b.profileDir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(b.profileDir, 0700); err != nil {
		return err
	}
	options := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	options = append(options,
		chromedp.UserDataDir(b.profileDir),
		chromedp.Flag("headless", false),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-notifications", true),
	)
	allocator, allocCancel := chromedp.NewExecAllocator(context.Background(), options...)
	browser, browserStop := chromedp.NewContext(allocator)
	if err := initializeXHSPage(ctx, browser, browserStop); err != nil {
		browserStop()
		allocCancel()
		return fmt.Errorf("无法打开小红书登录浏览器: %w", err)
	}
	b.allocCancel, b.browser, b.browserStop = allocCancel, browser, browserStop
	return nil
}

// The first Run owns the target event loop. It must use the persistent page
// context; cancelling a timeout wrapper after initialization kills that loop.
func initializeXHSPage(ctx, page context.Context, cancel context.CancelFunc) error {
	timer := time.AfterFunc(15*time.Second, cancel)
	stop := context.AfterFunc(ctx, cancel)
	err := chromedp.Run(page)
	timer.Stop()
	stop()
	return err
}

func (b *xhsBrowser) pageLocked(ctx context.Context) (context.Context, error) {
	if b.page != nil && b.page.Err() == nil {
		return b.page, nil
	}
	if b.pageStop != nil {
		b.pageStop()
	}
	b.page, b.pageStop = chromedp.NewContext(b.browser)
	if err := initializeXHSPage(ctx, b.page, b.pageStop); err != nil {
		b.pageStop()
		b.page, b.pageStop = nil, nil
		return nil, err
	}
	return b.page, nil
}

func (b *xhsBrowser) login(ctx context.Context) error {
	if err := b.start(ctx); err != nil {
		return err
	}
	b.mu.Lock()
	page, err := b.pageLocked(ctx)
	b.mu.Unlock()
	if err != nil {
		return err
	}
	operation, cancel := context.WithTimeout(page, 40*time.Second)
	stop := context.AfterFunc(ctx, cancel)
	defer func() { stop(); cancel() }()
	err = chromedp.Run(operation, chromedp.Navigate("https://www.xiaohongshu.com/explore"), chromedp.WaitReady("body", chromedp.ByQuery))
	if err != nil {
		b.mu.Lock()
		if b.page == page && b.pageStop != nil {
			b.pageStop()
			b.page, b.pageStop = nil, nil
		}
		b.mu.Unlock()
	}
	return err
}

func (b *xhsBrowser) logout(ctx context.Context) error {
	b.mu.Lock()
	if b.browser == nil || b.browser.Err() != nil {
		b.mu.Unlock()
		return nil
	}
	page, err := b.pageLocked(ctx)
	b.mu.Unlock()
	if err != nil {
		return err
	}
	operation, cancel := context.WithTimeout(page, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	return chromedp.Run(operation, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.ClearBrowserCookies().Do(ctx)
	}))
}

func (b *xhsBrowser) close() {
	b.mu.Lock()
	browser := b.browser
	pageStop, browserStop, allocCancel := b.pageStop, b.browserStop, b.allocCancel
	b.page, b.browser = nil, nil
	b.pageStop, b.browserStop, b.allocCancel = nil, nil, nil
	b.mu.Unlock()
	if pageStop != nil {
		pageStop()
	}
	if browser != nil && browser.Err() == nil {
		shutdown, cancel := context.WithTimeout(browser, 3*time.Second)
		_ = chromedp.Cancel(shutdown)
		cancel()
	}
	if browserStop != nil {
		browserStop()
	}
	if allocCancel != nil {
		allocCancel()
	}
}

func (a *App) Close() {
	a.CancelXHSLogin("")
	a.xhsBrowser.close()
	if a.stateDB != nil {
		_ = a.stateDB.Close()
	}
}

func (a *App) LogoutXHS(ctx context.Context) error {
	a.CancelXHSLogin("")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	a.xhsBrowser.mu.Lock()
	started := a.xhsBrowser.browser != nil
	a.xhsBrowser.mu.Unlock()
	if started {
		if err := a.xhsBrowser.logout(ctx); err != nil {
			return err
		}
	} else if err := os.RemoveAll(a.xhsBrowser.profileDir); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(filepath.Dir(a.statePath), "xiaohongshu.cookies.txt")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return a.deleteCredential(PlatformXiaohongshu)
}

func (b *xhsBrowser) cookieSnapshot(ctx context.Context) ([]*network.Cookie, error) {
	b.mu.Lock()
	browser := b.browser
	b.mu.Unlock()
	if browser == nil || browser.Err() != nil {
		return nil, errors.New("小红书登录浏览器尚未启动")
	}
	operation, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Storage.getCookies is a browser command, independent of any tab lifetime.
	cookies, err := storage.GetCookies().Do(cdp.WithExecutor(operation, chromedp.FromContext(browser).Browser))
	if err != nil {
		return nil, fmt.Errorf("读取浏览器 Cookie 失败: %w", err)
	}
	return cookies, nil
}

func xhsAccountFromCookies(cookies []*network.Cookie) XHSAccount {
	haveA1, haveSession := false, false
	for _, cookie := range cookies {
		if !isXHSCookieDomain(cookie.Domain) {
			continue
		}
		if cookie.Name == "a1" && cookie.Value != "" {
			haveA1 = true
		}
		if cookie.Name == "web_session" && cookie.Value != "" {
			haveSession = true
		}
	}
	if !haveA1 || !haveSession {
		return XHSAccount{State: "logged_out", Message: "请在小红书登录窗口扫码登录"}
	}
	return XHSAccount{State: "logged_in", Message: "已登录"}
}

type XHSAccount struct {
	State          string `json:"state"`
	Message        string `json:"message"`
	ID             string `json:"id,omitempty"`
	RedID          string `json:"redID,omitempty"`
	Name           string `json:"name,omitempty"`
	Avatar         string `json:"avatar,omitempty"`
	ProfileMessage string `json:"profileMessage,omitempty"`
}

type xhsImage struct {
	URLDefault string `json:"url_default"`
	URLPre     string `json:"url_pre"`
	URL        string `json:"url"`
	InfoList   []struct {
		Scene string `json:"image_scene"`
		URL   string `json:"url"`
	} `json:"info_list"`
}

func (image xhsImage) archiveURL() string {
	candidates := []string{image.URLDefault, image.URL}
	for _, variant := range image.InfoList {
		if variant.Scene == "WB_DFT" {
			candidates = append(candidates, variant.URL)
		}
	}
	for _, variant := range image.InfoList {
		candidates = append(candidates, variant.URL)
	}
	candidates = append(candidates, image.URLPre)
	for _, candidate := range candidates {
		if safe := secureXHSImageURL(candidate); safe != "" {
			return safe
		}
	}
	return ""
}

type xhsListNote struct {
	Time         int64      `json:"time"`
	ID           string     `json:"id"`
	NoteID       string     `json:"note_id"`
	XsecToken    string     `json:"xsec_token"`
	Type         string     `json:"type"`
	DisplayTitle string     `json:"display_title"`
	Title        string     `json:"title"`
	Desc         string     `json:"desc"`
	Cover        xhsImage   `json:"cover"`
	ImageList    []xhsImage `json:"image_list"`
	User         struct {
		Nickname string `json:"nickname"`
		Avatar   string `json:"avatar"`
	} `json:"user"`
	NoteCard *xhsListNote `json:"note_card"`
}

func parseXHSListNote(raw json.RawMessage) (Entry, string, string, error) {
	var note xhsListNote
	if err := json.Unmarshal(raw, &note); err != nil {
		return Entry{}, "", "", errors.New("小红书笔记条目格式变化，已停止扫描")
	}
	if note.NoteCard != nil {
		card := *note.NoteCard
		if card.ID == "" {
			card.ID = note.ID
		}
		if card.NoteID == "" {
			card.NoteID = note.NoteID
		}
		if card.XsecToken == "" {
			card.XsecToken = note.XsecToken
		}
		note = card
	}
	id := note.NoteID
	if id == "" {
		id = note.ID
	}
	if id == "" {
		return Entry{}, "", "", errors.New("小红书笔记缺少 ID，已停止扫描")
	}
	title := note.DisplayTitle
	if title == "" {
		title = note.Title
	}
	if title == "" {
		title = note.Desc
	}
	if title == "" {
		title = id
	}
	cover := note.Cover.archiveURL()
	kind := ContentImagePost
	if strings.EqualFold(note.Type, "video") {
		kind = ContentVideo
	}
	images := []string{}
	for _, image := range note.ImageList {
		imageURL := image.archiveURL()
		if imageURL == "" {
			return Entry{}, "", "", errors.New("小红书图片地址缺失或不受支持，已停止归档")
		}
		images = append(images, imageURL)
	}
	if cover == "" && len(images) > 0 {
		cover = images[0]
	}
	query := url.Values{"xsec_source": {"pc_user"}}
	if note.XsecToken != "" {
		query.Set("xsec_token", note.XsecToken)
	}
	entryURL := "https://www.xiaohongshu.com/explore/" + url.PathEscape(id) + "?" + query.Encode()
	return Entry{PublishedAt: publicationTime(note.Time), ID: id, Title: title, URL: entryURL, Thumbnail: secureXHSImageURL(cover), Content: note.Desc, Images: images, Kind: kind}, note.User.Nickname, secureXHSImageURL(note.User.Avatar), nil
}

func secureXHSImageURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Port() != "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if u.Scheme == "http" {
		u.Scheme = "https"
	}
	if u.Scheme != "https" || !strings.HasSuffix(host, ".xhscdn.com") {
		return ""
	}
	return u.String()
}

func (b *xhsBrowser) cookieFile(ctx context.Context) ([]byte, error) {
	cookies, err := b.cookieSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return xhsCookieFileFromCookies(cookies)
}

func xhsCookieFileFromCookies(cookies []*network.Cookie) ([]byte, error) {
	haveA1, haveSession := false, false
	for _, cookie := range cookies {
		domain := strings.TrimPrefix(cookie.Domain, ".")
		if !isXHSCookieDomain(domain) {
			continue
		}
		if cookie.Name == "a1" && cookie.Value != "" {
			haveA1 = true
		}
		if cookie.Name == "web_session" && cookie.Value != "" {
			haveSession = true
		}
	}
	if !haveA1 || !haveSession {
		return nil, errors.New("请先登录小红书")
	}
	var out strings.Builder
	out.WriteString("# Netscape HTTP Cookie File\n")
	for _, cookie := range cookies {
		domain := cookie.Domain
		if !isXHSCookieDomain(domain) || strings.ContainsAny(cookie.Name+cookie.Value, "\r\n\t;") {
			continue
		}
		expires := int64(cookie.Expires)
		if cookie.Session || expires <= 0 {
			expires = 0
		}
		subdomains, secure := "FALSE", "FALSE"
		if strings.HasPrefix(cookie.Domain, ".") {
			subdomains = "TRUE"
		}
		if cookie.Secure {
			secure = "TRUE"
		}
		fmt.Fprintf(&out, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n", domain, subdomains, cookie.Path, secure, expires, cookie.Name, cookie.Value)
	}
	return []byte(out.String()), nil
}

func isXHSCookieDomain(domain string) bool {
	domain = strings.TrimPrefix(strings.ToLower(domain), ".")
	return domain == "xiaohongshu.com" || strings.HasSuffix(domain, ".xiaohongshu.com")
}
