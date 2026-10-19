package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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

func (a *App) bilibiliCookies(ctx context.Context, s Settings) (string, error) {
	if s.CookiesFile != "" {
		header, err := cookiesForBilibili(s.CookiesFile)
		if err != nil {
			return "", err
		}
		if header == "" {
			return "", errors.New("Cookie 文件中没有未过期的 B 站 Cookie")
		}
		return s.CookiesFile, nil
	}
	a.guestMu.Lock()
	defer a.guestMu.Unlock()
	routeHash := sha256.Sum256([]byte(proxyURL(s)))
	path := filepath.Join(filepath.Dir(a.statePath), fmt.Sprintf("bilibili-guest-%x.cookies.txt", routeHash[:6]))
	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < 24*time.Hour {
		header, err := cookiesForBilibili(path)
		if err == nil && strings.Contains(header, "buvid3=") {
			return path, nil
		}
	}
	client, err := a.clientFor(s)
	if err != nil {
		return "", err
	}
	defer client.CloseIdleConnections()
	jar, _ := cookiejar.New(nil)
	client.Jar = jar
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.bilibili.com/", nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("初始化 B 站访客会话失败: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("B 站主页 HTTP %d，无法初始化访客会话", resp.StatusCode)
	}
	apiURL, _ := url.Parse("https://api.bilibili.com/")
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, "https://api.bilibili.com/x/frontend/finger/spi", nil)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	resp, err = client.Do(req)
	if err != nil {
		return "", fmt.Errorf("初始化 B 站访客标识失败: %w", err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return "", fmt.Errorf("B 站访客接口 HTTP %d", resp.StatusCode)
	}
	var visitor struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			B3 string `json:"b_3"`
			B4 string `json:"b_4"`
		} `json:"data"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&visitor)
	resp.Body.Close()
	if decodeErr != nil {
		return "", fmt.Errorf("B 站访客响应无效: %w", decodeErr)
	}
	if visitor.Code != 0 {
		return "", fmt.Errorf("B 站访客接口错误 %d: %s", visitor.Code, visitor.Message)
	}
	for name, value := range map[string]string{"buvid3": visitor.Data.B3, "buvid4": visitor.Data.B4} {
		if value != "" {
			jar.SetCookies(apiURL, []*http.Cookie{{Name: name, Value: value, Domain: ".bilibili.com", Path: "/", Secure: true}})
		}
	}
	var data strings.Builder
	data.WriteString("# Netscape HTTP Cookie File\n# BiliFetch: server-issued anonymous visitor session; no account login.\n")
	haveVisitor := false
	for _, cookie := range jar.Cookies(apiURL) {
		if cookie.Name == "buvid3" {
			haveVisitor = true
		}
		if strings.ContainsAny(cookie.Name+cookie.Value, "\r\n\t") {
			continue
		}
		fmt.Fprintf(&data, ".bilibili.com\tTRUE\t/\tTRUE\t%s\t%s\t%s\n", strconv.FormatInt(time.Now().Add(24*time.Hour).Unix(), 10), cookie.Name, cookie.Value)
	}
	if !haveVisitor {
		return "", errors.New("B 站未下发访客 Cookie，请在设置中配置有效 Cookie")
	}
	file, err := os.CreateTemp(filepath.Dir(a.statePath), ".guest-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.WriteString(data.String())
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err = os.Rename(file.Name(), path); err != nil {
		return "", err
	}
	return path, nil
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
			c.FailureCount++
			delay := time.Duration(c.Interval) * time.Minute
			if isRiskError(message) {
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

// A private copy prevents concurrent yt-dlp processes from rewriting the shared
// login/visitor cookie jar or modifying the user's exported credentials.
func (a *App) downloadOptions(ctx context.Context, bilibili bool, s Settings) ([]string, func(), error) {
	if err := validateProxy(proxyURL(s)); err != nil {
		return nil, func() {}, err
	}
	args := []string{"--proxy", proxyURL(s)}
	source := s.CookiesFile
	if bilibili {
		var err error
		source, err = a.bilibiliCookies(ctx, s)
		if err != nil {
			return nil, func() {}, err
		}
		args = append(args, "--user-agent", userAgent)
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
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.state.Tasks {
		t := &a.state.Tasks[i]
		if t.ID != taskID {
			continue
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
