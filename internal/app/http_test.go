package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetricsAPIReportsRealResources(t *testing.T) {
	a := testApp(t)
	h := a.Handler(context.Background(), "", "")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("资源接口应可用，得到 %d: %s", w.Code, w.Body.String())
	}
	var result struct {
		CollectedAt string `json:"collectedAt"`
		CPU         struct {
			Cores int `json:"cores"`
		} `json:"cpu"`
		Memory struct {
			Total uint64 `json:"total"`
		} `json:"memory"`
		Disk struct {
			Path      string `json:"path"`
			Total     uint64 `json:"total"`
			Available uint64 `json:"available"`
		} `json:"disk"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.CollectedAt == "" || result.CPU.Cores < 1 || result.Memory.Total == 0 {
		t.Fatalf("应返回真实采样时间、CPU 和内存，得到 %+v", result)
	}
	if result.Disk.Path != a.downloadDir || result.Disk.Total == 0 || result.Disk.Available > result.Disk.Total {
		t.Fatalf("磁盘应对应下载目录所在文件系统，得到 %+v", result.Disk)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("实时资源不能被浏览器缓存")
	}
	second := httptest.NewRecorder()
	h.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	if second.Body.String() != w.Body.String() {
		t.Fatal("高频请求应复用短期采样，不能重复扫描系统")
	}
}

func TestAPIValidationAndSameOrigin(t *testing.T) {
	a := testApp(t)
	h := a.Handler(context.Background(), "", "")
	cases := []struct {
		method, path, body, origin string
		want                       int
	}{
		{"GET", "/api/state", "", "", 200},
		{"POST", "/api/creators", `{"url":"123","interval":10,"quality":"best","watchDynamics":true}`, "", 201},
		{"POST", "/api/creators", `{"url":"123","interval":10,"quality":"best"}`, "", 400},
		{"POST", "/api/creators", `{"url":"http://localhost","interval":10,"quality":"best"}`, "", 400},
		{"POST", "/api/creators", `{}`, "https://evil.test", 403},
		{"POST", "/api/creators", `{"url":"123","interval":10,"quality":"best","surprise":true}`, "", 400},
		{"GET", "/api/missing", "", "", 404},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%s %s: got %d want %d: %s", tc.method, tc.path, w.Code, tc.want, w.Body.String())
		}
	}
}
func TestSafeFileRejectsEscapesAndSymlinks(t *testing.T) {
	a := testApp(t)
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.safeFile(outside); err == nil {
		t.Fatal("应拒绝下载目录外文件")
	}
	link := filepath.Join(a.downloadDir, "symlink")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := a.safeFile(link); err == nil {
		t.Fatal("应拒绝符号链接逃逸")
	}
}
func TestFileAPIHasTaskScopedAccess(t *testing.T) {
	a := testApp(t)
	file := filepath.Join(a.downloadDir, "test.mp4")
	if err := os.WriteFile(file, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	a.state.Tasks = append(a.state.Tasks, Task{ID: "test", Status: "completed", File: file})
	h := a.Handler(context.Background(), "", "")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tasks/test/files/0", nil))
	if w.Code != 200 || w.Body.String() != "video" {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestThumbnailAPIUsesArchivedImages(t *testing.T) {
	a := testApp(t)
	video := filepath.Join(a.downloadDir, "video.mp4")
	cover := filepath.Join(a.downloadDir, "video.webp")
	image := filepath.Join(a.downloadDir, "image-01.jpg")
	for _, path := range []string{cover, image} {
		if err := os.WriteFile(path, []byte("local image"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	a.state.Tasks = []Task{
		{ID: "video", Kind: "video", Status: "completed", File: video, Thumbnail: "https://example.invalid/remote.jpg"},
		{ID: "dynamic", Kind: "dynamic", Status: "completed", Files: []string{image}},
		{ID: "queued", Kind: "video", Status: "queued", File: video},
		{ID: "missing", Kind: "video", Status: "completed", File: filepath.Join(a.downloadDir, "missing.mp4")},
		{ID: "text", Kind: "dynamic", Status: "completed"},
	}
	h := a.Handler(context.Background(), "", "")
	for _, id := range []string{"video", "dynamic", "queued", "missing", "text", "unknown"} {
		t.Run(id, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tasks/"+id+"/thumbnail", nil))
			if id == "video" || id == "dynamic" {
				if w.Code != 200 || w.Body.String() != "local image" || !strings.HasPrefix(w.Header().Get("Content-Type"), "image/") {
					t.Fatalf("应直接返回本地图片，得到 %d %s", w.Code, w.Body.String())
				}
			} else if w.Code != 404 {
				t.Fatalf("没有可用归档封面时应返回 404，得到 %d", w.Code)
			}
		})
	}
}

func TestThumbnailAPIRejectsEscapesAndNonImages(t *testing.T) {
	a := testApp(t)
	outside := filepath.Join(t.TempDir(), "secret.jpg")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(a.downloadDir, "linked.jpg")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	text := filepath.Join(a.downloadDir, "content.md")
	if err := os.WriteFile(text, []byte("正文不能作为图片"), 0644); err != nil {
		t.Fatal(err)
	}
	a.state.Tasks = []Task{
		{ID: "outside", Kind: "dynamic", Status: "completed", Files: []string{outside}},
		{ID: "link", Kind: "video", Status: "completed", File: filepath.Join(a.downloadDir, "linked.mp4")},
		{ID: "text", Kind: "dynamic", Status: "completed", Files: []string{text}},
	}
	h := a.Handler(context.Background(), "", "")
	for _, id := range []string{"outside", "link", "text"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tasks/"+id+"/thumbnail", nil))
		if w.Code != 404 || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("不应暴露归档外文件或非图片：%s 得到 %d", id, w.Code)
		}
	}
}
