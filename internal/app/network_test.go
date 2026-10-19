package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func settingsWithRate(t *testing.T, a *App, rate float64) Settings {
	t.Helper()
	data, _ := json.Marshal(a.Snapshot().Settings)
	var fields map[string]any
	_ = json.Unmarshal(data, &fields)
	fields["maxDownloadSpeed"] = rate
	data, _ = json.Marshal(fields)
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	return settings
}

func TestNetworkSettingsValidationAndPersistence(t *testing.T) {
	for _, proxy := range []string{"", "http://127.0.0.1:7897", "https://proxy.example:443", "socks5://127.0.0.1:1080", "socks5h://user:pass@127.0.0.1:1080"} {
		if err := validateProxy(proxy); err != nil {
			t.Errorf("应支持代理协议 %q: %v", proxy, err)
		}
	}
	for _, proxy := range []string{"ftp://proxy.example", "http://", "http://proxy.example/path", "http://proxy.example/?x=1"} {
		if validateProxy(proxy) == nil {
			t.Errorf("应拒绝无效代理 %q", proxy)
		}
	}
	a := testApp(t)
	data, _ := json.Marshal(a.Snapshot().Settings)
	var fields map[string]any
	_ = json.Unmarshal(data, &fields)
	fields["proxyURL"] = "socks5h://127.0.0.1:1080"
	handler := a.Handler(context.Background(), "", "")
	for _, rate := range []float64{1.25, 0, -1, 0.00001, 100001} {
		fields["maxDownloadSpeed"] = rate
		body, _ := json.Marshal(fields)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("PUT", "/api/settings", bytes.NewReader(body)))
		valid := rate == 0 || rate == 1.25
		if valid && w.Code != 200 {
			t.Fatalf("有效网络设置无法保存: HTTP %d %s", w.Code, w.Body.String())
		}
		if !valid && w.Code != 400 {
			t.Fatalf("无效限速应拒绝: %v HTTP %d", rate, w.Code)
		}
		if valid {
			reopened, err := New(filepath.Dir(a.statePath), a.downloadDir, a.binary)
			if err != nil {
				t.Fatal(err)
			}
			saved, _ := json.Marshal(reopened.Snapshot().Settings)
			var got map[string]any
			_ = json.Unmarshal(saved, &got)
			if got["maxDownloadSpeed"] != rate || got["proxyURL"] != "socks5h://127.0.0.1:1080" {
				t.Fatalf("网络设置没有持久化: %s", saved)
			}
		}
	}
}

func TestVideoDownloadReceivesProxyAndRate(t *testing.T) {
	for _, rate := range []float64{1.25, 0} {
		a := testApp(t)
		binary, _ := fakeDownloader(t)
		arguments := filepath.Join(t.TempDir(), "args.txt")
		script, _ := os.ReadFile(binary)
		script = bytes.Replace(script, []byte("case \"$*\" in"), []byte("printf '%s\\n' \"$@\" > '"+arguments+"'\ncase \"$*\" in"), 1)
		if err := os.WriteFile(binary, script, 0700); err != nil {
			t.Fatal(err)
		}
		a.binary = binary
		s := settingsWithRate(t, a, rate)
		s.ProxyURL = "socks5h://127.0.0.1:1080"
		_, _, _, err := a.downloadVideo(context.Background(), Task{ID: "video", CreatorID: "up", URL: "https://www.youtube.com/watch?v=fixture", Quality: "best"}, s)
		if err != nil {
			t.Fatal(err)
		}
		args, _ := os.ReadFile(arguments)
		if !strings.Contains(string(args), "--proxy\nsocks5h://127.0.0.1:1080\n") {
			t.Fatal("视频下载未使用配置代理")
		}
		if rate > 0 && !strings.Contains(string(args), "--limit-rate\n1250000\n") {
			t.Fatal("视频下载未收到准确的字节限速")
		}
		if rate == 0 && strings.Contains(string(args), "--limit-rate") {
			t.Fatal("不限速时不能向 yt-dlp 传入限速")
		}
	}
}

func TestDynamicDownloadRateAndCancellation(t *testing.T) {
	a := testApp(t)
	a.state.Settings = settingsWithRate(t, a, 0.1)
	a.httpClient = &http.Client{Timeout: 40 * time.Millisecond, Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/jpeg"}}, Body: io.NopCloser(strings.NewReader(strings.Repeat("a", 10000)))}, nil
	})}
	start := time.Now()
	_, files, _, err := a.downloadDynamic(context.Background(), Task{ID: "paced", CreatorID: "up", Images: []string{"https://i0.hdslb.com/a.jpg", "https://i0.hdslb.com/b.jpg"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || time.Since(start) < 180*time.Millisecond {
		t.Fatal("图文下载没有遵循单任务限速")
	}
	data, _ := os.ReadFile(files[0])
	if len(data) != 10000 {
		t.Fatal("限速不能截断图片")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start = time.Now()
	_, _, _, err = a.downloadDynamic(ctx, Task{ID: "cancel", CreatorID: "up", Images: []string{"https://i0.hdslb.com/c.jpg"}})
	if err == nil || time.Since(start) > 500*time.Millisecond {
		t.Fatal("取消必须打断限速等待")
	}
	if _, err := os.Stat(filepath.Join(a.downloadDir, "up", "cancel", "image-01.jpg.part")); !os.IsNotExist(err) {
		t.Fatal("取消后应清理不完整图片")
	}
}
