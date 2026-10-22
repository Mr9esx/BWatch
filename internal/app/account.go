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
	header, _, err := a.bilibiliCookieHeader(ctx, s)
	if err != nil {
		if strings.Contains(err.Error(), "请先") {
			return BilibiliAccount{State: "logged_out", Message: "未登录"}
		}
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
	key := sha256.Sum256([]byte(proxyURL(s) + "\x00" + header))
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

// Profile retrieval is independent of local credential availability. A failed
// profile request must not gate subscriptions or start a login browser.
type xhsAccountCache struct {
	key     [32]byte
	until   time.Time
	account XHSAccount
}

func (a *App) XHSAccount(ctx context.Context) XHSAccount {
	result := a.SavedPlatformAccount(PlatformXiaohongshu)
	if result.State != "logged_in" {
		return result
	}
	credential, err := a.loadCredential(PlatformXiaohongshu)
	if err != nil {
		return XHSAccount{State: "unavailable", Message: "登录凭据不可读，请重新登录"}
	}
	key := sha256.Sum256(append([]byte(proxyURL(a.Snapshot().Settings)+"\x00"), credential...))
	a.xhsAccountMu.Lock()
	defer a.xhsAccountMu.Unlock()
	if a.xhsAccount.key == key {
		if time.Now().Before(a.xhsAccount.until) {
			return a.xhsAccount.account
		}
		result = a.xhsAccount.account
		result.ProfileMessage = ""
	}
	ttl := 5 * time.Minute
	defer func() { a.xhsAccount = xhsAccountCache{key: key, until: time.Now().Add(ttl), account: result} }()
	if a.platformBlocked(PlatformXiaohongshu, time.Now()) {
		result.ProfileMessage = "平台冷却中，稍后刷新账号资料"
		ttl = time.Minute
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	var profile XHSAccount
	if err := a.callXHSAPI(ctx, xhsAPIRequest{Operation: "account"}, &profile); err != nil {
		result.ProfileMessage = "账号资料获取失败，稍后可刷新；订阅仍使用已保存凭据"
		ttl = time.Minute
		_ = a.transaction(func() error { a.blockPlatformLocked(PlatformXiaohongshu, err.Error(), time.Now()); return nil })
		return result
	}
	if !xhsProfileID.MatchString(profile.ID) || strings.TrimSpace(profile.Name) == "" {
		result.ProfileMessage = "平台未返回有效的账号资料，请稍后刷新"
		ttl = time.Minute
		return result
	}
	result.ID, result.RedID, result.Name, result.Avatar = profile.ID, profile.RedID, profile.Name, secureXHSImageURL(profile.Avatar)
	return result
}
