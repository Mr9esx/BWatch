package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type LoginStatus struct {
	ID          string    `json:"id"`
	URL         string    `json:"url,omitempty"`
	ExpiresAt   time.Time `json:"expiresAt"`
	State       string    `json:"state"`
	Message     string    `json:"message"`
	CookiesFile string    `json:"cookiesFile,omitempty"`
}
type loginSession struct {
	LoginStatus
	Key        string
	Client     *http.Client
	LastPollAt time.Time
	Cookies    map[string]*http.Cookie
}

func (a *App) StartLogin(ctx context.Context) (LoginStatus, error) {
	settings := a.Snapshot().Settings
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	if a.login != nil && (a.login.State == "waiting_scan" || a.login.State == "waiting_confirm") && time.Until(a.login.ExpiresAt) > 20*time.Second {
		return a.login.LoginStatus, nil
	}
	client, err := a.clientFor(settings)
	if err != nil {
		return LoginStatus{}, err
	}
	jar, _ := cookiejar.New(nil)
	client.Jar = jar
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://passport.bilibili.com/x/passport-login/web/qrcode/generate", nil)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	resp, err := client.Do(req)
	if err != nil {
		client.CloseIdleConnections()
		return LoginStatus{}, errors.New("无法连接 B 站登录接口，请检查网络与代理")
	}
	defer resp.Body.Close()
	var payload struct {
		Code int `json:"code"`
		Data struct {
			URL string `json:"url"`
			Key string `json:"qrcode_key"`
		} `json:"data"`
	}
	if resp.StatusCode != 200 {
		return LoginStatus{}, fmt.Errorf("B 站登录接口 HTTP %d，请检查网络与代理", resp.StatusCode)
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload) != nil || payload.Code != 0 || payload.Data.Key == "" {
		return LoginStatus{}, errors.New("B 站未返回有效二维码，请稍后重试")
	}
	target, err := url.Parse(payload.Data.URL)
	if err != nil || target.Scheme != "https" || (target.Host != "passport.bilibili.com" && target.Host != "account.bilibili.com") || target.User != nil {
		return LoginStatus{}, errors.New("B 站返回了非预期的登录地址")
	}
	if a.login != nil {
		a.login.Client.CloseIdleConnections()
	}
	a.login = &loginSession{LoginStatus: LoginStatus{ID: id(), URL: payload.Data.URL, ExpiresAt: time.Now().Add(180 * time.Second), State: "waiting_scan", Message: "请使用哔哩哔哩 App 扫码"}, Key: payload.Data.Key, Client: client, Cookies: map[string]*http.Cookie{}}
	return a.login.LoginStatus, nil
}
func (a *App) PollLogin(ctx context.Context, sessionID string) (LoginStatus, error) {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()
	session := a.login
	if session == nil || session.ID != sessionID {
		return LoginStatus{}, errors.New("登录二维码不存在，请重新生成")
	}
	if session.State == "success" || session.State == "expired" {
		return session.LoginStatus, nil
	}
	if time.Now().After(session.ExpiresAt) {
		session.State = "expired"
		session.Message = "二维码已过期，请重新生成"
		session.Client.CloseIdleConnections()
		return session.LoginStatus, nil
	}
	if time.Since(session.LastPollAt) < 2*time.Second {
		return session.LoginStatus, nil
	}
	session.LastPollAt = time.Now()
	endpoint := "https://passport.bilibili.com/x/passport-login/web/qrcode/poll?qrcode_key=" + url.QueryEscape(session.Key)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	resp, err := session.Client.Do(req)
	if err != nil {
		return session.LoginStatus, errors.New("无法查询扫码状态，请检查网络与代理")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return session.LoginStatus, fmt.Errorf("B 站扫码状态接口 HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Code int `json:"code"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload) != nil || payload.Code != 0 {
		return session.LoginStatus, errors.New("B 站扫码状态响应异常")
	}
	for _, cookie := range resp.Cookies() {
		session.Cookies[cookie.Name] = cookie
	}
	switch payload.Data.Code {
	case 86101:
		session.State = "waiting_scan"
		session.Message = "等待手机扫码"
	case 86090:
		session.State = "waiting_confirm"
		session.Message = "已扫码，请在手机上确认登录"
	case 86038:
		session.State = "expired"
		session.Message = "二维码已过期，请重新生成"
	case 0:
		apiURL, _ := url.Parse("https://api.bilibili.com/")
		cookies := session.Client.Jar.Cookies(apiURL)
		haveLogin := false
		var data strings.Builder
		data.WriteString("# Netscape HTTP Cookie File\n# BiliFetch: Bilibili QR login. Keep private.\n")
		for _, cookie := range cookies {
			if cookie.Name == "SESSDATA" && cookie.Value != "" {
				haveLogin = true
			}
			if strings.ContainsAny(cookie.Name+cookie.Value, "\r\n\t") {
				continue
			}
			expiry := time.Now().Add(24 * time.Hour)
			if original := session.Cookies[cookie.Name]; original != nil {
				if !original.Expires.IsZero() {
					expiry = original.Expires
				} else if original.MaxAge > 0 {
					expiry = time.Now().Add(time.Duration(original.MaxAge) * time.Second)
				}
			}
			fmt.Fprintf(&data, ".bilibili.com\tTRUE\t/\tTRUE\t%d\t%s\t%s\n", expiry.Unix(), cookie.Name, cookie.Value)
		}
		if !haveLogin {
			return session.LoginStatus, errors.New("手机已确认，但 B 站尚未返回有效登录 Cookie，请重新扫码")
		}
		path := filepath.Join(filepath.Dir(a.statePath), "bilibili-login-"+session.ID+".cookies.txt")
		if err := os.WriteFile(path, []byte(data.String()), 0600); err != nil {
			return session.LoginStatus, errors.New("保存登录 Cookie 失败，请检查数据目录权限")
		}
		err := a.transaction(func() error {
			a.state.Settings.CookiesFile = path
			for i := range a.state.Creators {
				if a.state.Creators[i].Platform == "bilibili" {
					a.state.Creators[i].NextCheckAt = nil
					a.state.Creators[i].LastChecked = nil
				}
			}
			a.event("success", "B 站扫码登录成功，Cookie 已保存在本机")
			return nil
		})
		if err != nil {
			os.Remove(path)
			return session.LoginStatus, errors.New("保存登录设置失败，请检查数据目录权限")
		}
		session.State = "success"
		session.Message = "登录成功，已用于视频与图文监听"
		session.CookiesFile = path
		session.URL = ""
		session.Key = ""
		session.Client.CloseIdleConnections()
	default:
		return session.LoginStatus, fmt.Errorf("B 站扫码返回状态 %d，请重新扫码", payload.Data.Code)
	}
	return session.LoginStatus, nil
}
