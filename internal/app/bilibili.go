package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func proxyURL(s Settings) string {
	if s.ProxyURL != "" {
		return s.ProxyURL
	}
	return os.Getenv("BILIFETCH_PROXY")
}
func validateProxy(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("代理地址应为 HTTP、HTTPS 或 SOCKS5 URL，例如 http://127.0.0.1:7897")
	}
	return nil
}
func (a *App) clientFor(s Settings) (*http.Client, error) {
	raw := proxyURL(s)
	if err := validateProxy(raw); err != nil {
		return nil, err
	}
	client := *a.httpClient
	// Preserve injected transports for tests; production creates a per-operation
	// transport so concurrent settings changes cannot mutate an in-flight request.
	transport, ok := client.Transport.(*http.Transport)
	if client.Transport == nil {
		transport = http.DefaultTransport.(*http.Transport)
		ok = true
	}
	if ok {
		copy := transport.Clone()
		copy.Proxy = nil
		if raw != "" {
			u, _ := url.Parse(raw)
			copy.Proxy = http.ProxyURL(u)
		}
		client.Transport = copy
	}
	return &client, nil
}

func (a *App) bilibiliCookieHeader(_ context.Context, _ Settings) (string, []byte, error) {
	data, err := a.loadCredential(PlatformBilibili)
	if err != nil {
		return "", nil, err
	}
	header, err := cookiesForBilibiliData(data)
	if err != nil {
		return "", nil, err
	}
	if header == "" {
		return "", nil, errors.New("登录凭据中没有未过期的 B 站 Cookie")
	}
	hasSession := false
	for _, pair := range strings.Split(header, "; ") {
		if strings.HasPrefix(pair, "SESSDATA=") && pair != "SESSDATA=" {
			hasSession = true
			break
		}
	}
	if !hasSession {
		return "", nil, errors.New("登录凭据没有有效登录态，请重新扫码登录 B 站")
	}
	return header, data, nil
}

func (a *App) recordScanOutcome(creatorID, message string, now time.Time) error {
	return a.transaction(func() error {
		for i := range a.state.Creators {
			c := &a.state.Creators[i]
			if c.ID != creatorID {
				continue
			}
			c.LastChecked = &now
			c.LastError = message
			c.NextCheckAt = nil
			if message == "" {
				c.FailureCount = 0
				a.event("success", "「"+c.Name+"」更新检查完成")
				return nil
			}
			if c.Platform == PlatformXiaohongshu && isPlatformRiskError(c.Platform, message) {
				a.blockPlatformLocked(c.Platform, message, now)
			}
			c.FailureCount++
			delay := time.Duration(c.Interval) * time.Minute
			if isPlatformRiskError(c.Platform, message) {
				delay = 30 * time.Minute
				for n := 1; n < c.FailureCount && delay < 6*time.Hour; n++ {
					delay *= 2
				}
				if delay > 6*time.Hour {
					delay = 6 * time.Hour
				}
			}
			next := now.Add(delay)
			c.NextCheckAt = &next
			a.event("error", "「"+c.Name+"」检查失败: "+message+"；下次自动检查 "+next.Format(time.RFC3339))
			return nil
		}
		return nil
	})
}
func isRiskError(message string) bool {
	for _, token := range []string{"HTTP Error 412", "HTTP 412", "(412)", "HTTP Error 429", "HTTP 429", "-352", "(352)", "info: 352", "-401", "(401)"} {
		if strings.Contains(message, token) {
			return true
		}
	}
	return false
}

// yt-dlp accepts a Netscape cookie file, so write a short-lived private copy
// from the encrypted credential and remove it when the process ends.
func (a *App) downloadOptions(ctx context.Context, platform PlatformID, s Settings) ([]string, func(), error) {
	if err := validateProxy(proxyURL(s)); err != nil {
		return nil, func() {}, err
	}
	args := []string{"--proxy", proxyURL(s)}
	source := ""
	if platform == PlatformBilibili {
		_, data, err := a.bilibiliCookieHeader(ctx, s)
		if err != nil {
			return nil, func() {}, err
		}
		temp, cleanup, err := a.writeTemporaryCookieFile(data)
		if err != nil {
			return nil, func() {}, err
		}
		return append(args, "--user-agent", userAgent, "--cookies", temp), cleanup, nil
	}
	if platform == PlatformXiaohongshu {
		data, err := a.loadCredential(PlatformXiaohongshu)
		if err != nil {
			data, err = a.xhsBrowser.cookieFile(ctx)
			if err != nil {
				return nil, func() {}, err
			}
			if saveErr := a.saveCredential(PlatformXiaohongshu, data); saveErr != nil {
				return nil, func() {}, saveErr
			}
		}
		temp, cleanup, err := a.writeTemporaryCookieFile(data)
		if err != nil {
			return nil, func() {}, err
		}
		return append(args, "--user-agent", userAgent, "--cookies", temp), cleanup, nil
	}
	if source == "" {
		return args, func() {}, nil
	}
	file, err := os.Open(source)
	if err != nil {
		return nil, func() {}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil {
		return nil, func() {}, err
	}
	if len(data) > 2<<20 {
		return nil, func() {}, errors.New("Cookie 文件超过 2MB")
	}
	copy, err := os.CreateTemp(filepath.Dir(a.statePath), ".yt-cookies-*")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.Remove(copy.Name()) }
	if err = copy.Chmod(0600); err == nil {
		_, err = copy.Write(data)
	}
	closeErr := copy.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return append(args, "--cookies", copy.Name()), cleanup, nil
}
func (a *App) captureVideoMetadata(taskID, path string) {
	metadata := strings.TrimSuffix(path, filepath.Ext(path)) + ".info.json"
	file, err := os.Open(metadata)
	if err != nil {
		return
	}
	defer file.Close()
	var info struct {
		Title     string  `json:"title"`
		Thumbnail string  `json:"thumbnail"`
		Duration  float64 `json:"duration"`
		Uploader  string  `json:"uploader"`
	}
	if json.NewDecoder(io.LimitReader(file, 4<<20)).Decode(&info) != nil {
		return
	}
	publishedAt := a.localPublication(Task{Kind: ContentVideo, File: path})
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.state.Tasks {
		t := &a.state.Tasks[i]
		if t.ID != taskID {
			continue
		}
		if publishedAt != nil {
			t.PublishedAt = publishedAt
		}
		if info.Title != "" {
			t.Title = info.Title
		}
		if info.Thumbnail != "" {
			t.Thumbnail = info.Thumbnail
		}
		if info.Duration > 0 {
			t.Duration = info.Duration
		}
		if info.Uploader != "" {
			t.CreatorName = info.Uploader
		}
		return
	}
}
