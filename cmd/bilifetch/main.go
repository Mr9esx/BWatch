package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"bilifetch/internal/app"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func addLocalToolPaths() {
	paths := []string{}
	if workingDir, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(workingDir, ".tools", "venv", "bin"))
	}
	if runtime.GOOS == "darwin" {
		paths = append(paths, "/opt/homebrew/bin", "/usr/local/bin")
	}
	current := filepath.SplitList(os.Getenv("PATH"))
	seen := make(map[string]bool, len(paths)+len(current))
	joined := make([]string, 0, len(paths)+len(current))
	for _, path := range append(paths, current...) {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		joined = append(joined, path)
	}
	_ = os.Setenv("PATH", strings.Join(joined, string(os.PathListSeparator)))
}

func main() {
	addLocalToolPaths()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a, err := app.New(env("BILIFETCH_DATA", "data"), env("BILIFETCH_DOWNLOADS", "downloads"), env("BILIFETCH_YTDLP", "yt-dlp"))
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: env("BILIFETCH_ADDR", "127.0.0.1:8090"), Handler: a.Handler(ctx, env("BILIFETCH_WEB", "web/dist"), env("BILIFETCH_DEV_ORIGIN", "http://127.0.0.1:5173,http://localhost:5173")), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
	go a.Run(ctx)
	go func() {
		log.Printf("BiliFetch 已启动 http://%s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP 服务失败: %v", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
	if err := a.Drain(shutdown); err != nil {
		log.Printf("等待任务退出: %v", err)
	}
	a.Close()
}
