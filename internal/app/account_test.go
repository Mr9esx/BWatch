package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 捕获把 Cookie 文件当作有效登录、泄露凭据、重复请求及风控误判等回归。
func TestBilibiliAccountStatus(t *testing.T) {
	for _, tc := range []struct {
		name, cookie, body, state string
		status                    int
	}{
		{"未配置", "", "", "logged_out", 200},
		{"访客凭据", "buvid3", "", "expired", 200},
		{"已登录", "SESSDATA", `{"code":0,"data":{"isLogin":true,"mid":12345,"uname":"测试账号","face":"https://i0.hdslb.com/bfs/face/test.jpg"}}`, "logged_in", 200},
		{"登录过期", "SESSDATA", `{"code":-101,"message":"账号未登录"}`, "expired", 200},
		{"非登录响应", "SESSDATA", `{"code":0,"data":{"isLogin":false}}`, "expired", 200},
		{"风控", "SESSDATA", `{"code":-352}`, "unavailable", 200},
		{"HTTP风控", "SESSDATA", `{}`, "unavailable", 412},
		{"接口异常", "SESSDATA", `{"code":-500}`, "unavailable", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t)
			if tc.cookie != "" {
				path := filepath.Join(t.TempDir(), "cookies.txt")
				if err := os.WriteFile(path, []byte(".bilibili.com\tTRUE\t/\tTRUE\t0\t"+tc.cookie+"\tfixture-secret\n"), 0600); err != nil {
					t.Fatal(err)
				}
				a.state.Settings.CookiesFile = path
			}
			calls := 0
			a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != "https://api.bilibili.com/x/web-interface/nav" || r.Header.Get("Cookie") != "SESSDATA=fixture-secret" {
					t.Error("应携带凭据请求账号接口")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			handler := a.Handler(context.Background(), "", "")
			for i := 0; i < 2; i++ {
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/bilibili/account", nil))
				var result struct {
					State  string `json:"state"`
					UID    int64  `json:"uid"`
					Name   string `json:"name"`
					Avatar string `json:"avatar"`
				}
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.State != tc.state {
					t.Fatalf("账号状态不符: HTTP %d %s", w.Code, w.Body.String())
				}
				if tc.state == "logged_in" && (result.UID != 12345 || result.Name != "测试账号" || result.Avatar != "https://i0.hdslb.com/bfs/face/test.jpg") {
					t.Fatalf("缺少账号资料: %+v", result)
				}
				if strings.Contains(w.Body.String(), "fixture-secret") || strings.Contains(w.Body.String(), "cookies.txt") {
					t.Fatal("不能向前端返回凭据")
				}
			}
			wantCalls := 1
			if tc.cookie != "SESSDATA" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatalf("账号请求次数 %d，期望 %d", calls, wantCalls)
			}
			if (tc.status == 412 || tc.body == `{"code":-352}`) && !future(a.Snapshot().BilibiliBlockedUntil, time.Now()) {
				t.Fatal("风控必须进入全局冷却")
			}
		})
	}
}

func TestBilibiliAccountCacheTracksCredentialsAndCooldown(t *testing.T) {
	a := testApp(t)
	path := filepath.Join(t.TempDir(), "cookies.txt")
	writeCookie := func(value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(".bilibili.com\tTRUE\t/\tTRUE\t0\tSESSDATA\t"+value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeCookie("first")
	a.state.Settings.CookiesFile = path
	var calls atomic.Int32
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		name := "甲"
		if r.Header.Get("Cookie") == "SESSDATA=second" {
			name = "乙"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"isLogin":true,"mid":123,"uname":"` + name + `","face":"https://untrusted.example/avatar"}}`))}, nil
	})}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := a.BilibiliAccount(context.Background())
			if got.Name != "甲" || got.Avatar != "" {
				t.Errorf("账号解析或头像域名校验失败: %+v", got)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("并发查询应合并请求")
	}
	writeCookie("second")
	if got := a.BilibiliAccount(context.Background()); got.Name != "乙" {
		t.Fatal("同一路径替换 Cookie 后不能显示旧账号")
	}
	a.account.until = time.Now().Add(-time.Second)
	until := time.Now().Add(time.Hour)
	a.state.BilibiliBlockedUntil = &until
	if got := a.BilibiliAccount(context.Background()); got.State != "unavailable" || got.Name != "乙" {
		t.Fatal("冷却中应保留资料并标记尚未验证")
	}
	if calls.Load() != 2 {
		t.Fatal("冷却中不能请求 B 站")
	}
}
