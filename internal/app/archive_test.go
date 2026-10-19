package app

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDynamicArchiveContainsTextImagesAndMetadata(t *testing.T) {
	a := testApp(t)
	dir := filepath.Join(a.downloadDir, "archive")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"content.md": "正文", "image-01.jpg": "image bytes", "metadata.json": "{}"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	a.state.Tasks = []Task{{ID: "dynamic", Kind: "dynamic", VideoID: "123", Status: "completed", File: filepath.Join(dir, "content.md"), Files: []string{filepath.Join(dir, "image-01.jpg")}}}
	w := httptest.NewRecorder()
	a.Handler(context.Background(), "", "").ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks/dynamic/archive", nil))
	if w.Code != 200 {
		t.Fatalf("归档下载失败 %d: %s", w.Code, w.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range reader.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		got[f.Name] = string(data)
	}
	if got["content.md"] != "正文" || got["image-01.jpg"] != "image bytes" || got["metadata.json"] != "{}" || len(got) != 3 {
		t.Fatalf("图文包应包含完整产物: %+v", got)
	}
}
