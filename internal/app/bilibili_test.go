package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGuestSessionUsesServerIssuedCookiesWithoutLogin(t *testing.T) {
	a := testApp(t)
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/x/frontend/finger/spi" {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"b_3":"server-issued-visitor","b_4":"server-issued-visitor4"}}`))}, nil
		}
		if r.URL.String() != "https://www.bilibili.com/" {
			t.Fatalf("访客初始化只能请求正常主页: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": []string{"buvid3=server-issued-visitor; Domain=.bilibili.com; Path=/; Max-Age=86400; Secure"}}, Body: io.NopCloser(strings.NewReader("<html>首页</html>"))}, nil
	})}
	path, err := a.bilibiliCookies(context.Background(), Settings{})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "buvid3\tserver-issued-visitor") || strings.Contains(string(content), "SESSDATA") {
		t.Fatal("应保存真实访客 Cookie，不能伪造登录态")
	}
	if _, err := a.bilibiliCookies(context.Background(), Settings{}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("Cookie 文件应仅对当前用户可读写")
	}
}
func TestCookieHeaderIgnoresExpiredAndForeignCookies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.txt")
	data := "# Netscape HTTP Cookie File\n.bilibili.com\tTRUE\t/\tTRUE\t1\tSESSDATA\texpired\n.evil.test\tTRUE\t/\tFALSE\t0\tother\tsecret\n#HttpOnly_.bilibili.com\tTRUE\t/\tTRUE\t0\tbuvid3\tvisitor\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	header, err := cookiesForBilibili(path)
	if err != nil {
		t.Fatal(err)
	}
	if header != "buvid3=visitor" {
		t.Fatalf("不应发送过期或外站 Cookie: %q", header)
	}
}
func TestRiskBackoffGrowsAndResets(t *testing.T) {
	a := testApp(t)
	c, err := a.AddCreator(CreatorInput{URL: "123", Interval: 1, Quality: "best"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := a.recordScanOutcome(c.ID, "视频: HTTP Error 412: Precondition Failed", now); err != nil {
		t.Fatal(err)
	}
	c = a.Snapshot().Creators[0]
	if c.NextCheckAt == nil || c.NextCheckAt.Sub(now) < 30*time.Minute || c.FailureCount != 1 {
		t.Fatal("412 应退避至少30分钟")
	}
	if err := a.recordScanOutcome(c.ID, "动态接口错误 -352", now); err != nil {
		t.Fatal(err)
	}
	c = a.Snapshot().Creators[0]
	if c.NextCheckAt.Sub(now) < time.Hour {
		t.Fatal("连续风控应增加退避")
	}
	if err := a.recordScanOutcome(c.ID, "", now); err != nil {
		t.Fatal(err)
	}
	c = a.Snapshot().Creators[0]
	if c.NextCheckAt != nil || c.FailureCount != 0 {
		t.Fatal("成功后应恢复正常间隔")
	}
}

func TestGuestInitializationDoesNotHideRiskResponses(t *testing.T) {
	for _, status := range []int{429, 200} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			a := testApp(t)
			c := Creator{URL: "https://space.bilibili.com/123/video"}
			calls := 0
			a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host == "www.bilibili.com" {
					return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": []string{"buvid3=real-visitor; Domain=.bilibili.com; Path=/; Secure"}}, Body: io.NopCloser(strings.NewReader("home"))}, nil
				}
				if r.URL.Path == "/x/frontend/finger/spi" {
					return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":-352,"message":"risk"}`))}, nil
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"has_more":false,"items":[]}}`))}, nil
			})}
			_, err := a.fetchBilibiliPage(context.Background(), c, a.Snapshot().Settings, "")
			if err == nil || !isRiskError(err.Error()) || calls != 2 || !future(a.Snapshot().BilibiliBlockedUntil, time.Now()) {
				t.Fatalf("访客初始化风控不能被吞掉，err=%v calls=%d", err, calls)
			}
		})
	}
}
