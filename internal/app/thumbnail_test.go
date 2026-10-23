package app

import (
	"bytes"
	"context"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestXHSVideoThumbnailUsesVideoFrameAndCache(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("需要 FFmpeg")
	}
	a := testApp(t)
	video := filepath.Join(a.downloadDir, "clip.mp4")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x180:rate=2", "-t", "2", "-c:v", "mpeg4", video).CombinedOutput(); err != nil {
		t.Fatalf("生成测试视频: %v %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(a.downloadDir, "clip.jpg"), []byte("平台低清预览"), 0644); err != nil {
		t.Fatal(err)
	}
	a.state.Tasks = []Task{{ID: "xhs", Platform: PlatformXiaohongshu, Kind: ContentVideo, Status: "completed", File: video, Duration: 2}}
	h := a.Handler(context.Background(), "", "")
	read := func() []byte {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tasks/xhs/thumbnail", nil))
		if w.Code != 200 {
			t.Fatalf("thumbnail status %d", w.Code)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
		if err != nil || cfg.Width != 320 || cfg.Height != 180 {
			t.Fatalf("应使用原视频画面且不放大: %+v %v", cfg, err)
		}
		return w.Body.Bytes()
	}
	first := read()
	// 视频暂不可解码时，已有缓存仍应可用，不重复运行 FFmpeg。
	if err := os.WriteFile(video, []byte("video temporarily unavailable"), 0644); err != nil {
		t.Fatal(err)
	}
	if second := read(); !bytes.Equal(first, second) {
		t.Fatal("重复请求未复用高清封面缓存")
	}
}

func TestXHSVideoThumbnailFallsBackToArchivedCover(t *testing.T) {
	a := testApp(t)
	video := filepath.Join(a.downloadDir, "broken.mp4")
	if err := os.WriteFile(video, []byte("invalid video"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.downloadDir, "broken.jpg"), []byte("archived cover"), 0644); err != nil {
		t.Fatal(err)
	}
	a.state.Tasks = []Task{{ID: "xhs", Platform: PlatformXiaohongshu, Kind: ContentVideo, Status: "completed", File: video}}
	w := httptest.NewRecorder()
	a.Handler(context.Background(), "", "").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tasks/xhs/thumbnail", nil))
	if w.Code != 200 || w.Body.String() != "archived cover" {
		t.Fatalf("生成失败应保留旧封面: %d %s", w.Code, w.Body.String())
	}
}
