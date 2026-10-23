package app

import (
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMediaMetadataReadsArchivedFiles(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("需要 ffprobe")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("需要 FFmpeg")
	}
	a := testApp(t)
	video := filepath.Join(a.downloadDir, "clip.mp4")
	out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x180:rate=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "1", "-c:v", "mpeg4", "-c:a", "aac", video).CombinedOutput()
	if err != nil {
		t.Fatalf("生成视频: %v %s", err, out)
	}
	photo := filepath.Join(a.downloadDir, "photo.png")
	f, err := os.Create(photo)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 48, 32))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	a.state.Tasks = []Task{
		{ID: "video", Kind: ContentVideo, Status: "completed", File: video},
		{ID: "photo", Kind: ContentImagePost, Status: "completed", File: "manifest.json", Files: []string{photo}},
	}
	h := a.Handler(context.Background(), "", "")
	read := func(path string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("媒体参数接口应返回 200，得到 %d: %s", w.Code, w.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	v := read("/api/tasks/video/files/0/metadata")
	stream := v["video"].(map[string]any)
	audio := v["audio"].([]any)[0].(map[string]any)
	if stream["codec"] != "mpeg4" || stream["width"] != float64(320) || stream["height"] != float64(180) || stream["frameRate"] != float64(24) || audio["codec"] != "aac" || audio["sampleRate"] != float64(48000) || audio["channels"] != float64(1) || v["duration"].(float64) < 1 || v["bitRate"].(float64) <= 0 || audio["bitRate"].(float64) <= 0 {
		t.Fatalf("视频实际参数不正确: %+v", v)
	}
	silent := filepath.Join(a.downloadDir, "silent.mp4")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-i", video, "-an", "-c:v", "copy", silent).CombinedOutput(); err != nil {
		t.Fatalf("生成无声视频: %v %s", err, out)
	}
	a.state.Tasks = append(a.state.Tasks, Task{ID: "silent", Kind: ContentVideo, Status: "completed", File: silent})
	if got := read("/api/tasks/silent/files/0/metadata"); len(got["audio"].([]any)) != 0 || got["video"] == nil {
		t.Fatalf("无声视频仍应提供视频参数和空音轨列表: %+v", got)
	}
	p := read("/api/tasks/photo/files/1/metadata")
	stream = p["video"].(map[string]any)
	stat, _ := os.Stat(photo)
	if p["fileName"] != "photo.png" || p["size"] != float64(stat.Size()) || stream["codec"] != "png" || stream["width"] != float64(48) || stream["height"] != float64(32) || len(p["audio"].([]any)) != 0 {
		t.Fatalf("图片实际参数不正确: %+v", p)
	}
}

func TestMediaNumbers(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  float64
	}{{"30000/1001", 29.97002997002997}, {"1.25", 1.25}, {"N/A", 0}, {"0/0", 0}, {"1/0", 0}, {"NaN", 0}, {"-10", 0}} {
		if got := mediaNumber(tc.value); got != tc.want {
			t.Errorf("%s: 得到 %v，期望 %v", tc.value, got, tc.want)
		}
	}
}

func TestMediaMetadataRejectsUnavailableOrUnsafeFiles(t *testing.T) {
	a := testApp(t)
	outside := filepath.Join(t.TempDir(), "private.mp4")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(a.downloadDir, "link.mp4")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	a.state.Tasks = []Task{
		{ID: "outside", Kind: ContentVideo, Status: "completed", File: outside},
		{ID: "symlink", Kind: ContentVideo, Status: "completed", File: link},
		{ID: "missing", Kind: ContentVideo, Status: "completed", File: filepath.Join(a.downloadDir, "missing.mp4")},
		{ID: "pending", Kind: ContentVideo, Status: "downloading", File: outside},
		{ID: "text", Kind: ContentImagePost, Status: "completed", File: outside},
	}
	h := a.Handler(context.Background(), "", "")
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"outside/files/0", 404}, {"symlink/files/0", 404}, {"missing/files/0", 404}, {"pending/files/0", 404}, {"unknown/files/0", 404}, {"text/files/0", 404}, {"outside/files/1", 404}, {"outside/files/-1", 400}, {"outside/files/invalid", 400},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks/"+tc.path+"/metadata", nil))
		if w.Code != tc.status {
			t.Errorf("%s: 得到 %d，期望 %d", tc.path, w.Code, tc.status)
		}
	}
}

func TestMediaMetadataMissingProbeAndInvalidMedia(t *testing.T) {
	a := testApp(t)
	path := filepath.Join(a.downloadDir, "broken.mp4")
	if err := os.WriteFile(path, []byte("invalid media"), 0600); err != nil {
		t.Fatal(err)
	}
	a.state.Tasks = []Task{{ID: "broken", Kind: ContentVideo, Status: "completed", File: path}}
	h := a.Handler(context.Background(), "", "")
	if _, err := exec.LookPath("ffprobe"); err == nil {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks/broken/files/0/metadata", nil))
		if w.Code != 422 {
			t.Fatalf("损坏文件应报告不可解析: %d %s", w.Code, w.Body.String())
		}
	}
	t.Setenv("PATH", t.TempDir())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks/broken/files/0/metadata", nil))
	if w.Code != 503 {
		t.Fatalf("缺少 ffprobe 应报告依赖不可用: %d %s", w.Code, w.Body.String())
	}
}
