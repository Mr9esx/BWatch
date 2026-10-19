package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type BilibiliAccount struct {
	State   string `json:"state"`
	UID     int64  `json:"uid,omitempty"`
	Name    string `json:"name,omitempty"`
	Avatar  string `json:"avatar,omitempty"`
	Message string `json:"message,omitempty"`
}
type accountCache struct {
	key     [32]byte
	until   time.Time
	account BilibiliAccount
}

func (a *App) BilibiliAccount(ctx context.Context) (result BilibiliAccount) {
	s := a.Snapshot().Settings
	if s.CookiesFile == "" {
		return BilibiliAccount{State: "logged_out", Message: "未登录"}
	}
	header, err := cookiesForBilibili(s.CookiesFile)
	if err != nil {
		return BilibiliAccount{State: "unavailable", Message: "登录凭据不可读"}
	}
	haveSession := false
	for _, pair := range strings.Split(header, "; ") {
		if strings.HasPrefix(pair, "SESSDATA=") && pair != "SESSDATA=" {
			haveSession = true
		}
	}
	if !haveSession {
		return BilibiliAccount{State: "expired", Message: "登录已失效"}
	}
	key := sha256.Sum256([]byte(s.CookiesFile + "\x00" + proxyURL(s) + "\x00" + header))
	// ponytail: 单账号共用锁合并多标签页查询；多账号支持时再按账号拆分。
	a.accountMu.Lock()
	defer a.accountMu.Unlock()
	if a.account.key == key && time.Now().Before(a.account.until) {
		return a.account.account
	}
	result = BilibiliAccount{State: "unavailable", Message: "暂时无法验证登录"}
	if a.account.key == key {
		result.UID, result.Name, result.Avatar = a.account.account.UID, a.account.account.Name, a.account.account.Avatar
	}
	defer func() {
		ttl := time.Minute
		if result.State == "logged_in" || result.State == "expired" {
			ttl = 5 * time.Minute
		}
		a.account = accountCache{key: key, until: time.Now().Add(ttl), account: result}
	}()
	if a.bilibiliBlocked(time.Now()) {
		result.Message = "B 站冷却中，暂未验证"
		return
	}
	client, err := a.clientFor(s)
	if err != nil {
		return
	}
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.bilibili.com/x/web-interface/nav", nil)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	req.Header.Set("Cookie", header)
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var payload struct {
		Code int `json:"code"`
		Data struct {
			IsLogin bool   `json:"isLogin"`
			MID     int64  `json:"mid"`
			Name    string `json:"uname"`
			Face    string `json:"face"`
		} `json:"data"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload)
	message := fmt.Sprintf("B 站账号接口 HTTP %d，code %d", resp.StatusCode, payload.Code)
	if isRiskError(message) {
		result.Message = "B 站冷却中，暂未验证"
		if a.transaction(func() error { a.blockBilibiliLocked(message, time.Now()); return nil }) != nil {
			result.Message = "B 站风控，保存冷却失败"
		}
		return
	}
	if resp.StatusCode != http.StatusOK || decodeErr != nil {
		return
	}
	if payload.Code == -101 || (payload.Code == 0 && !payload.Data.IsLogin) {
		return BilibiliAccount{State: "expired", Message: "登录已失效"}
	}
	if payload.Code != 0 || payload.Data.MID <= 0 || payload.Data.Name == "" {
		return
	}
	avatar := ""
	if u, err := url.Parse(payload.Data.Face); err == nil && u.Scheme == "https" && imageHost(u.Hostname()) && u.User == nil {
		avatar = u.String()
	}
	return BilibiliAccount{State: "logged_in", UID: payload.Data.MID, Name: payload.Data.Name, Avatar: avatar, Message: "已登录"}
}
