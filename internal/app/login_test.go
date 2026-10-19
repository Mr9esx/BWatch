package app

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestQRLoginSavesCookiesOnlyAfterPhoneConfirmation(t *testing.T) {
	a := testApp(t)
	confirmed := false
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"code":0,"data":{"url":"https://account.bilibili.com/h5/account-h5/auth/scan-web?qrcode_key=fixture-key","qrcode_key":"fixture-key"}}`
		headers := http.Header{}
		if strings.Contains(r.URL.Path, "poll") {
			if !confirmed {
				body = `{"code":0,"data":{"code":86101,"message":"未扫码"}}`
			} else {
				body = `{"code":0,"data":{"code":0,"message":""}}`
				headers.Add("Set-Cookie", "SESSDATA=fixture-login; Domain=.bilibili.com; Path=/; Secure; HttpOnly")
				headers.Add("Set-Cookie", "DedeUserID=123; Domain=.bilibili.com; Path=/; Secure")
			}
		}
		return &http.Response{StatusCode: 200, Header: headers, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	login, err := a.StartLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a.Snapshot().Settings.CookiesFile != "" {
		t.Fatal("生成二维码不能提前标记登录")
	}
	status, err := a.PollLogin(context.Background(), login.ID)
	if err != nil || status.State != "waiting_scan" {
		t.Fatalf("应等候手机扫码: %+v %v", status, err)
	}
	if a.Snapshot().Settings.CookiesFile != "" {
		t.Fatal("未确认不能保存登录态")
	}
	confirmed = true
	a.login.LastPollAt = a.login.LastPollAt.Add(-3_000_000_000)
	status, err = a.PollLogin(context.Background(), login.ID)
	if err != nil || status.State != "success" {
		t.Fatalf("手机确认后应保存登录: %+v %v", status, err)
	}
	path := a.Snapshot().Settings.CookiesFile
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "SESSDATA\tfixture-login") {
		t.Fatal("应保存可供 yt-dlp 使用的 Netscape Cookie")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("登录 Cookie 文件权限必须是0600")
	}
	if strings.Contains(status.URL, "fixture-login") || status.CookiesFile == "" {
		t.Fatal("前端只应收到文件路径与登录状态")
	}
}
func TestLoginRejectsExpiredAndUnknownSession(t *testing.T) {
	a := testApp(t)
	if _, err := a.PollLogin(context.Background(), "unknown"); err == nil {
		t.Fatal("未知二维码不能轮询")
	}
}
