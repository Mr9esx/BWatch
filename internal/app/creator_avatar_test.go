package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func creatorAvatar(t *testing.T, c Creator) string {
	t.Helper()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var profile struct {
		Avatar string `json:"avatar"`
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatal(err)
	}
	return profile.Avatar
}

func TestCreatorAvatarIsFetchedWithNameAndPersisted(t *testing.T) {
	for _, tc := range []struct {
		name    string
		modules string
		videos  bool
		history bool
	}{
		{"网页作者与视频监听", `{"module_author":{"name":"UP","face":"http://i0.hdslb.com/bfs/face/up.jpg"}}`, true, false},
		{"桌面作者与图文监听", `[{"module_author":{"name":"UP","face":"https://i0.hdslb.com/bfs/face/up.jpg"}}]`, false, false},
		{"桌面嵌套作者与历史拉取", `[{"module_author":{"user":{"name":"UP","face":"https://i0.hdslb.com/bfs/face/up.jpg"}}}]`, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t)
			useTestCookies(t, a)
			body := fmt.Sprintf(`{"code":0,"data":{"has_more":false,"items":[{"id_str":"post","type":"DYNAMIC_TYPE_WORD","modules":%s}]}}`, tc.modules)
			a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/x/space/navnum" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"video":0,"opus":0}}`))}, nil
				}
				if r.URL.Path != "/x/polymer/web-dynamic/desktop/v1/feed/space" {
					t.Fatalf("名称与头像应共用内容接口: %s", r.URL.Path)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			c, err := a.AddCreator(CreatorInput{URL: "123", Interval: 10, Quality: "best", WatchVideos: &tc.videos, WatchDynamics: !tc.videos})
			if err != nil {
				t.Fatal(err)
			}
			if tc.history {
				if err = a.StartHistory(c.ID, HistoryInput{Limit: 1, WatchVideos: true}); err == nil {
					err = a.ProcessHistoryPage(context.Background(), c.ID)
				}
			} else {
				err = a.ScanCreator(context.Background(), c.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			got := a.Snapshot().Creators[0]
			if got.Name != "UP" || creatorAvatar(t, got) != "https://i0.hdslb.com/bfs/face/up.jpg" {
				t.Fatalf("应同时保存名称与 HTTPS 头像: name=%q avatar=%q", got.Name, creatorAvatar(t, got))
			}
			// 后续响应缺少头像时，不能清空已有头像。
			body = `{"code":0,"data":{"items":[{"id_str":"next","modules":{"module_author":{"name":"UP"}}}]}}`
			if err := a.ScanCreator(context.Background(), c.ID); err != nil {
				t.Fatal(err)
			}
			reopened, err := New(filepath.Dir(a.statePath), a.downloadDir, a.binary)
			if err != nil {
				t.Fatal(err)
			}
			if got := creatorAvatar(t, reopened.Snapshot().Creators[0]); got != "https://i0.hdslb.com/bfs/face/up.jpg" {
				t.Fatalf("重启后应保留上次有效头像: %q", got)
			}
		})
	}
}

func TestCreatorAvatarRejectsUntrustedImageURLs(t *testing.T) {
	for _, face := range []string{"https://untrusted.example/avatar.jpg", "javascript:alert(1)", "https://user:secret@i0.hdslb.com/a.jpg", "https://i0.hdslb.com:8443/a.jpg"} {
		t.Run(face, func(t *testing.T) {
			a := testApp(t)
			useTestCookies(t, a)
			a.httpClient = &http.Client{Transport: testTransport(func(*http.Request) (*http.Response, error) {
				body := fmt.Sprintf(`{"code":0,"data":{"items":[{"modules":{"module_author":{"name":"UP","face":%q}}}]}}`, face)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			c, err := a.AddCreator(CreatorInput{URL: "123", Interval: 10, Quality: "best"})
			if err == nil {
				err = a.ScanCreator(context.Background(), c.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			got := a.Snapshot().Creators[0]
			if got.Name != "UP" || creatorAvatar(t, got) != "" {
				t.Fatalf("无效头像不应影响名称或传给浏览器: %+v", got)
			}
		})
	}
}
