package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func apiError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return errors.New("请求数据无效: " + err.Error())
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("只允许一个 JSON 对象")
	}
	return nil
}

func (a *App) serveThumbnail(w http.ResponseWriter, r *http.Request) {
	for _, task := range a.Snapshot().Tasks {
		if task.ID != r.PathValue("id") || task.Status != "completed" {
			continue
		}
		var candidates []string
		if task.Kind == "dynamic" {
			if len(task.Files) > 0 {
				candidates = task.Files[:1]
			}
		} else if task.File != "" {
			// yt-dlp 的封面与最终视频同名，直接兼容已有归档，无需迁移状态。
			base := strings.TrimSuffix(task.File, filepath.Ext(task.File))
			for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp", ".avif", ".gif"} {
				candidates = append(candidates, base+ext)
			}
		}
		for _, candidate := range candidates {
			switch strings.ToLower(filepath.Ext(candidate)) {
			case ".jpg", ".jpeg", ".png", ".webp", ".avif", ".gif":
			default:
				continue
			}
			path, err := a.safeFile(candidate)
			if err != nil {
				continue
			}
			http.ServeFile(w, r, path)
			return
		}
		break
	}
	http.NotFound(w, r)
}

func (a *App) Handler(ctx context.Context, webDir, devOrigin string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/metrics", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, a.metrics.sample(ctx, a.downloadDir))
	})
	mux.HandleFunc("GET /api/bilibili/account", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, a.BilibiliAccount(r.Context()))
	})
	mux.HandleFunc("POST /api/creators/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		var in HistoryInput
		if err := decodeBody(w, r, &in); err != nil {
			apiError(w, 400, err)
			return
		}
		if err := a.StartHistory(r.PathValue("id"), in); err != nil {
			apiError(w, 400, err)
			return
		}
		writeJSON(w, 201, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/creators/{id}/history/{action}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.HistoryAction(r.PathValue("id"), r.PathValue("action")); err != nil {
			apiError(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/bilibili/login", func(w http.ResponseWriter, r *http.Request) {
		result, err := a.StartLogin(r.Context())
		if err != nil {
			apiError(w, 400, err)
			return
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("POST /api/bilibili/login/{id}/poll", func(w http.ResponseWriter, r *http.Request) {
		result, err := a.PollLogin(r.Context(), r.PathValue("id"))
		if err != nil {
			apiError(w, 400, err)
			return
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, a.Snapshot()) })
	mux.HandleFunc("GET /api/tasks/{id}/archive", a.serveArchive)
	mux.HandleFunc("GET /api/tasks/{id}/thumbnail", a.serveThumbnail)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		_, ytErr := exec.LookPath(a.binary)
		_, ffErr := exec.LookPath("ffmpeg")
		a.mu.Lock()
		checking := []string{}
		for id := range a.checking {
			checking = append(checking, id)
		}
		a.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true, "ytDlp": ytErr == nil, "ffmpeg": ffErr == nil, "downloadDir": a.downloadDir, "checking": checking, "version": "1.0.0"})
	})
	mux.HandleFunc("POST /api/creators", func(w http.ResponseWriter, r *http.Request) {
		var in CreatorInput
		if err := decodeBody(w, r, &in); err != nil {
			apiError(w, 400, err)
			return
		}
		c, err := a.AddCreator(in)
		if err != nil {
			apiError(w, 400, err)
			return
		}
		writeJSON(w, 201, c)
	})
	mux.HandleFunc("POST /api/creators/{id}/toggle", func(w http.ResponseWriter, r *http.Request) {
		if err := a.ToggleCreator(r.PathValue("id")); err != nil {
			apiError(w, 404, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("DELETE /api/creators/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.DeleteCreator(r.PathValue("id")); err != nil {
			apiError(w, 404, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/creators/{id}/check", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		found := false
		for _, c := range a.Snapshot().Creators {
			if c.ID == id {
				found = true
				if !c.Enabled {
					apiError(w, 400, errors.New("请先启用订阅"))
					return
				}
			}
		}
		if !found {
			apiError(w, 404, errors.New("订阅不存在"))
			return
		}
		go func() { _ = a.ScanCreator(ctx, id) }()
		writeJSON(w, 202, map[string]string{"message": "已开始检查"})
	})
	mux.HandleFunc("POST /api/check", func(w http.ResponseWriter, r *http.Request) {
		go func() {
			for _, c := range a.Snapshot().Creators {
				if c.Enabled && ctx.Err() == nil {
					_ = a.ScanCreator(ctx, c.ID)
				}
			}
		}()
		writeJSON(w, 202, map[string]string{"message": "已开始检查全部订阅"})
	})
	mux.HandleFunc("POST /api/tasks/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		if err := a.TaskAction(r.PathValue("id"), r.PathValue("action")); err != nil {
			apiError(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/tasks/{id}/files/{index}", func(w http.ResponseWriter, r *http.Request) {
		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil || index < 0 {
			apiError(w, 400, errors.New("文件索引无效"))
			return
		}
		for _, t := range a.Snapshot().Tasks {
			if t.ID != r.PathValue("id") || t.Status != "completed" {
				continue
			}
			paths := append([]string{t.File}, t.Files...)
			if index >= len(paths) {
				break
			}
			path, err := a.safeFile(paths[index])
			if err != nil {
				apiError(w, 404, errors.New("文件已移动或不存在"))
				return
			}
			if r.URL.Query().Get("download") == "1" {
				w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(filepath.Base(path))))
			}
			http.ServeFile(w, r, path)
			return
		}
		apiError(w, 404, errors.New("归档文件不存在"))
	})
	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var s Settings
		if err := decodeBody(w, r, &s); err != nil {
			apiError(w, 400, err)
			return
		}
		if err := a.UpdateSettings(s); err != nil {
			apiError(w, 400, err)
			return
		}
		writeJSON(w, 200, s)
	})
	mux.HandleFunc("POST /api/control", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Paused bool `json:"paused"`
		}
		if err := decodeBody(w, r, &in); err != nil {
			apiError(w, 400, err)
			return
		}
		if err := a.SetPaused(in.Paused); err != nil {
			apiError(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("DELETE /api/events", func(w http.ResponseWriter, r *http.Request) {
		if err := a.transaction(func() error { a.state.Events = []Event{}; return nil }); err != nil {
			apiError(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { apiError(w, 404, errors.New("接口不存在")) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method Not Allowed", 405)
			return
		}
		if webDir == "" {
			http.Error(w, "前端尚未构建，请执行 make build", 503)
			return
		}
		path := filepath.Join(webDir, filepath.Clean("/"+r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(webDir, "index.html"))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			origin := r.Header.Get("Origin")
			if origin != "" {
				u, err := url.Parse(origin)
				if err != nil || ((u.Host != r.Host || (u.Scheme != "https" && u.Scheme != "http")) && origin != devOrigin) {
					apiError(w, 403, errors.New("拒绝跨来源修改请求"))
					return
				}
			}
		}
		mux.ServeHTTP(w, r)
	})
}
