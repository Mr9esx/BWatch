package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// 小红书下载器可能保存高度压缩的预览封面，直接从本地视频生成一次高清封面。
func (a *App) xhsVideoThumbnail(ctx context.Context, task Task) (string, error) {
	video, err := a.safeFile(task.File)
	if err != nil {
		return "", err
	}
	// ponytail: 串行限制 FFmpeg 的资源占用；封面并发量增加时再改为按视频加锁。
	a.thumbnailMu.Lock()
	defer a.thumbnailMu.Unlock()
	cache := video + ".preview-v1.jpg"
	if safe, err := a.safeFile(cache); err == nil {
		return safe, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tmp, err := os.CreateTemp(filepath.Dir(video), ".preview-*.jpg")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	defer os.Remove(name)
	seek := 1.0
	if task.Duration > 0 && task.Duration < 3 {
		seek = task.Duration / 3
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-ss", strconv.FormatFloat(seek, 'f', 3, 64), "-i", video, "-frames:v", "1", "-vf", "scale='min(1280,iw)':-2", "-q:v", "2", "-threads", "1", "-update", "1", name)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	info, err := os.Stat(name)
	if err != nil {
		return "", err
	}
	if info.Size() == 0 {
		return "", os.ErrInvalid
	}
	if err := os.Rename(name, cache); err != nil {
		return "", err
	}
	return a.safeFile(cache)
}
