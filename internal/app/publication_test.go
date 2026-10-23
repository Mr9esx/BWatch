package app

import (
	"archive/zip"
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
)

func publicationJSON(t *testing.T, v any) string {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	return strings.Trim(string(result["publishedAt"]), `"`)
}

func TestPublicationFromPlatformFeeds(t *testing.T) {
	raw := json.RawMessage(`{"note_card":{"note_id":"note","type":"video","title":"笔记","time":1760000000000,"last_update_time":1790000000000}}`)
	entry, _, _, err := parseXHSListNote(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := publicationJSON(t, entry); got != "2025-10-09T08:53:20Z" {
		t.Fatalf("应保存原始发布时间而非编辑时间: %s", got)
	}
	feed, err := parseBilibiliFeed(strings.NewReader(`{"code":0,"data":{"items":[{"id_str":"post","type":"DYNAMIC_TYPE_WORD","modules":{"module_author":{"name":"UP","pub_ts":1760000000},"module_dynamic":{"desc":{"text":"正文"}}}}]}}`))
	if err != nil || len(feed.Dynamics) != 1 {
		t.Fatalf("feed: %+v %v", feed, err)
	}
	if got := publicationJSON(t, feed.Dynamics[0]); got != "2025-10-09T08:53:20Z" {
		t.Fatalf("B站发布时间: %s", got)
	}
}

func TestPublicationBackfillFromLocalMetadata(t *testing.T) {
	data, downloads := t.TempDir(), t.TempDir()
	a, err := New(data, downloads, "yt-dlp")
	if err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(downloads, "video.mp4")
	if err := os.WriteFile(video, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(downloads, "video.info.json"), []byte(`{"timestamp":1760000000}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := a.transaction(func() error {
		return json.Unmarshal([]byte(`{"tasks":[{"id":"video","kind":"video","platform":"bilibili","status":"completed","file":`+string(mustJSON(video))+`},{"id":"unknown","kind":"image_post","status":"completed"}]}`), &a.state)
	}); err != nil {
		t.Fatal(err)
	}
	a.Close()
	b, err := New(data, downloads, "yt-dlp")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	tasks := b.Snapshot().Tasks
	if got := publicationJSON(t, tasks[0]); got != "2025-10-09T08:53:20Z" {
		t.Fatalf("旧视频应从下载元数据恢复: %s", got)
	}
	if got := publicationJSON(t, tasks[1]); got != "" && got != "null" {
		t.Fatalf("未知时间不能使用归档时间代替: %s", got)
	}
	w := httptest.NewRecorder()
	b.Handler(context.Background(), "", "").ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/tasks/video/publication", nil))
	if w.Code != 200 {
		t.Fatalf("已有时间应直接复用: %d %s", w.Code, w.Body.String())
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func TestPublicationSourceDetailsMatchTask(t *testing.T) {
	for _, tc := range []struct {
		name      string
		kind      ContentKind
		body      string
		wantError bool
	}{
		{"video", ContentVideo, `{"code":0,"data":{"bvid":"source","pubdate":1760000000}}`, false},
		{"wrong-video", ContentVideo, `{"code":0,"data":{"bvid":"other","pubdate":1760000000}}`, true},
		{"dynamic", ContentImagePost, `{"code":0,"data":{"item":{"id_str":"source","modules":{"module_author":{"pub_ts":1760000000}}}}}`, false},
		{"wrong-dynamic", ContentImagePost, `{"code":0,"data":{"item":{"id_str":"other","modules":{"module_author":{"pub_ts":1760000000}}}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t)
			if err := a.saveCredential(PlatformBilibili, []byte(".bilibili.com\tTRUE\t/\tTRUE\t0\tSESSDATA\tfixture\n")); err != nil {
				t.Fatal(err)
			}
			a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Scheme != "https" || r.URL.Host != "api.bilibili.com" || r.Header.Get("Cookie") != "SESSDATA=fixture" {
					t.Fatal("详情请求未使用官方接口和已保存凭据")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			at, err := a.fetchSourcePublication(context.Background(), Task{Platform: PlatformBilibili, Kind: tc.kind, VideoID: "source"})
			if (err != nil) != tc.wantError {
				t.Fatalf("错误: %v", err)
			}
			if !tc.wantError && (at == nil || at.Unix() != 1760000000) {
				t.Fatalf("发布时间: %v", at)
			}
		})
	}
}

func TestPublicationIncludedInOldArchiveExport(t *testing.T) {
	a := testApp(t)
	dir := filepath.Join(a.downloadDir, "old-post")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := filepath.Join(dir, "content.md")
	if err := os.WriteFile(content, []byte("# 旧正文\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(`{"custom":"保留","id":"old"}`), 0644); err != nil {
		t.Fatal(err)
	}
	a.state.Tasks = []Task{{ID: "old", VideoID: "post", Kind: ContentImagePost, Status: "completed", File: content, PublishedAt: publicationTime(1760000000)}}
	w := httptest.NewRecorder()
	a.Handler(context.Background(), "", "").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tasks/old/archive", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	z, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte("2025-10-09T08:53:20Z")) {
			t.Fatalf("%s 导出缺少补齐时间: %s", f.Name, body)
		}
		if f.Name == "metadata.json" && !bytes.Contains(body, []byte("保留")) {
			t.Fatal("原元数据字段丢失")
		}
	}
}
