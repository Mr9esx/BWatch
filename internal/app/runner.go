package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type cappedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errors.New("外部命令输出超过限制")
	}
	return b.Buffer.Write(p)
}
func (a *App) ScanCreator(ctx context.Context, creatorID string) error {
	a.mu.Lock()
	if a.checking[creatorID] {
		a.mu.Unlock()
		return errors.New("正在检查，请稍后")
	}
	var creator *Creator
	for _, c := range a.state.Creators {
		if c.ID == creatorID {
			copy := c
			creator = &copy
			break
		}
	}
	if creator == nil {
		a.mu.Unlock()
		return errors.New("订阅不存在")
	}
	if !creator.Enabled {
		a.mu.Unlock()
		return errors.New("请先启用订阅")
	}
	if future(a.platformBlockedUntilLocked(creator.Platform), time.Now()) {
		a.mu.Unlock()
		return fmt.Errorf("%s正在冷却，请等待恢复", contentPlatformLabel(creator.Platform))
	}
	a.checking[creatorID] = true
	settings := a.state.Settings
	a.mu.Unlock()
	defer func() { a.mu.Lock(); delete(a.checking, creatorID); a.mu.Unlock() }()
	timeout := 3 * time.Minute
	if creator.Platform == PlatformXiaohongshu {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	failures := []string{}
	provider, ok := a.platforms.byPlatform(PlatformID(creator.Platform))
	if !ok {
		failures = append(failures, "不支持的平台: "+string(creator.Platform))
	} else {
		feed, scanErr := provider.Scan(ctx, a, *creator, settings)
		if scanErr != nil {
			failures = append(failures, contentPlatformLabel(PlatformID(creator.Platform))+"内容: "+scanErr.Error())
		} else {
			videos, imagePosts := []Entry{}, []Entry{}
			catalog := map[string]bool{}
			for _, entry := range feed.Entries {
				if entry.Kind == ContentVideo {
					catalog[string(ContentVideo)+":"+entry.ID] = true
					videos = append(videos, entry)
				} else if entry.Kind == ContentImagePost {
					catalog[string(ContentImagePost)+":"+entry.ID] = true
					imagePosts = append(imagePosts, entry)
				}
			}
			if err := a.recordPlatformCatalog(creatorID, catalog, feed.Complete); err != nil {
				return err
			}
			if creator.WatchVideos {
				if err := a.applyScan(creatorID, videos, feed.Name, ContentVideo, feed.Avatar); err != nil {
					failures = append(failures, "视频: "+err.Error())
				}
			}
			if creator.WatchDynamics {
				if err := a.applyScan(creatorID, imagePosts, feed.Name, ContentImagePost, feed.Avatar); err != nil {
					failures = append(failures, "图文: "+err.Error())
				}
			}
		}
	}
	scanErr := strings.Join(failures, "；")
	err := a.recordScanOutcome(creatorID, scanErr, time.Now())
	if err != nil {
		return err
	}
	if scanErr != "" {
		return errors.New(scanErr)
	}
	if creator.Platform == "bilibili" {
		if err := a.refreshContentTotals(ctx, *creator, settings); err != nil {
			log.Printf("刷新订阅总量失败: %v", err)
		}
	}
	return nil
}

func contentPlatformLabel(platform PlatformID) string {
	if platform == PlatformXiaohongshu {
		return "小红书"
	}
	return "B 站"
}

func (a *App) recordPlatformCatalog(creatorID string, entries map[string]bool, complete bool) error {
	return a.transaction(func() error {
		for i := range a.state.Creators {
			c := &a.state.Creators[i]
			if c.ID != creatorID || !c.Enabled {
				continue
			}
			if h := c.HistoryJob; h != nil && h.CatalogFromStart && (h.Status == "running" || h.Status == "paused" || h.Status == "failed") {
				mergeCatalog(&h.CatalogEntries, entries)
			}
			if complete {
				c.ContentCatalog = entries
				c.ContentCatalogComplete = true
			} else {
				mergeCatalog(&c.ContentCatalog, entries)
			}
			return nil
		}
		return errors.New("订阅不存在")
	})
}
func (a *App) scanVideos(ctx context.Context, c Creator, s Settings) ([]Entry, string, error) {
	args := []string{"--ignore-config", "--flat-playlist", "--dump-single-json", "--playlist-end", "50", "--socket-timeout", "20", "--retries", "1", "--no-warnings"}
	options, cleanup, err := a.downloadOptions(ctx, PlatformID(c.Platform), s)
	if err != nil {
		return nil, "", err
	}
	defer cleanup()
	args = append(args, options...)
	args = append(args, "--", c.URL)
	cmd := exec.CommandContext(ctx, a.binary, args...)
	configureProcess(cmd)
	out := &cappedBuffer{limit: 20 << 20}
	stderr := &cappedBuffer{limit: 64 << 10}
	cmd.Stdout = out
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return nil, "", fmt.Errorf("yt-dlp 解析失败: %s", commandError(err, stderr.String()))
	}
	var data struct {
		Title    string `json:"title"`
		Uploader string `json:"uploader"`
		Channel  string `json:"channel"`
		Entries  *[]struct {
			ID         string `json:"id"`
			Title      string `json:"title"`
			URL        string `json:"url"`
			WebURL     string `json:"webpage_url"`
			Thumbnail  string `json:"thumbnail"`
			Thumbnails []struct {
				URL string `json:"url"`
			} `json:"thumbnails"`
			Timestamp float64 `json:"timestamp"`
			Duration  float64 `json:"duration"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(out.Bytes(), &data); err != nil {
		return nil, "", fmt.Errorf("yt-dlp JSON 无效: %w", err)
	}
	if data.Entries == nil {
		return nil, "", errors.New("未返回频道视频列表，未建立基线")
	}
	entries := []Entry{}
	for _, e := range *data.Entries {
		if e.ID == "" {
			continue
		}
		videoURL := "https://www.bilibili.com/video/" + e.ID
		thumbnail := e.Thumbnail
		if thumbnail == "" && len(e.Thumbnails) > 0 {
			thumbnail = e.Thumbnails[0].URL
		}
		entries = append(entries, Entry{PublishedAt: publicationTime(int64(e.Timestamp)), ID: e.ID, Title: e.Title, URL: videoURL, Thumbnail: thumbnail, Duration: e.Duration})
	}
	name := data.Uploader
	if name == "" {
		name = data.Channel
	}
	return entries, name, nil
}
func commandError(err error, stderr string) string {
	message := strings.TrimSpace(stderr)
	if message == "" {
		message = err.Error()
	}
	message = sanitizeCookieFailure(message)
	runes := []rune(message)
	if len(runes) > 600 {
		message = string(runes[len(runes)-600:])
	}
	return message
}

// CookieJar includes the offending Cookie line in its traceback. Never save
// that line in task errors or events, since it may contain login credentials.
func sanitizeCookieFailure(message string) string {
	lower := strings.ToLower(message)
	if strings.Contains(lower, "invalid netscape format cookies file") || strings.Contains(lower, "cookieloaderror") {
		return "下载器无法读取 Cookie 文件，请重试下载；若仍失败，请重新扫码登录"
	}
	return message
}
func (a *App) updateProgress(taskID string, percent float64, speed, eta string, size int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.state.Tasks {
		t := &a.state.Tasks[i]
		if t.ID == taskID && t.Status == "downloading" {
			if percent >= 0 && percent <= 100 {
				t.Progress = percent
			}
			t.Speed = speed
			t.ETA = eta
			t.Bytes = size
			return
		}
	}
}
func (a *App) downloadVideo(ctx context.Context, t Task, s Settings) (string, []string, int64, error) {
	directory := filepath.Join(a.downloadDir, t.CreatorID, t.ID)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", nil, 0, err
	}
	format := "bv*+ba/b"
	args := []string{"--ignore-config", "--no-playlist", "--no-simulate", "--continue", "--no-overwrites", "--newline", "--progress", "--progress-delta", "1", "--progress-template", "download:BF_PROGRESS:%(progress._percent_str)s|%(progress._speed_str)s|%(progress._eta_str)s|%(progress.downloaded_bytes)s", "--print", "after_move:BF_FILE:%(filepath)s", "--socket-timeout", "20", "--retries", "3", "--fragment-retries", "3", "--windows-filenames", "--trim-filenames", "180", "--write-info-json", "--merge-output-format", "mp4", "-f", format, "-P", directory, "-o", "%(title).140B [%(id)s].%(ext)s"}
	platform := t.Platform
	if platform == "" {
		platform = PlatformXiaohongshu
		if strings.Contains(t.URL, "bilibili.com/") {
			platform = PlatformBilibili
		}
	}
	options, cleanup, err := a.downloadOptions(ctx, platform, s)
	if err != nil {
		return "", nil, 0, err
	}
	defer cleanup()
	args = append(args, options...)
	resolutionSort := "res"
	if rate := downloadBytesPerSecond(s); rate > 0 {
		args = append(args, "--limit-rate", strconv.FormatInt(rate, 10))
	}
	if t.Quality != "best" {
		resolutionSort += ":" + t.Quality
	}
	args = append(args, "--format-sort-force", "-S", resolutionSort)
	if isBilibiliTask(t) {
		args = append(args, "--sleep-requests", "2", "--retries", "0", "--fragment-retries", "1", "--extractor-retries", "0", "--concurrent-fragments", "1")
	}
	if s.SaveThumbnail {
		args = append(args, "--write-thumbnail")
	}
	if s.DownloadSubtitles {
		args = append(args, "--write-subs", "--sub-langs", "zh.*,en.*")
	}
	args = append(args, "--", t.URL)
	cmd := exec.CommandContext(ctx, a.binary, args...)
	configureProcess(cmd)
	if platform == PlatformXiaohongshu {
		a.configureXHSPython(cmd, true)
	}
	stderr := &cappedBuffer{limit: 256 << 10}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, 0, err
	}
	if err = cmd.Start(); err != nil {
		return "", nil, 0, err
	}
	path := ""
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "BF_FILE:") {
			path = strings.TrimSpace(strings.TrimPrefix(line, "BF_FILE:"))
		}
		if strings.HasPrefix(line, "BF_PROGRESS:") {
			parts := strings.Split(strings.TrimPrefix(line, "BF_PROGRESS:"), "|")
			if len(parts) == 4 {
				percent, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[0]), "%")), 64)
				size, _ := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
				a.updateProgress(t.ID, percent, strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2]), size)
			}
		}
	}
	scanErr := scanner.Err()
	if scanErr != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	err = cmd.Wait()
	if err != nil {
		return "", nil, 0, errors.New(commandError(err, stderr.String()))
	}
	if scanErr != nil {
		return "", nil, 0, scanErr
	}
	safe, err := a.safeFile(path)
	if err != nil {
		return "", nil, 0, fmt.Errorf("未找到下载产物: %w", err)
	}
	info, err := os.Stat(safe)
	if err != nil {
		return "", nil, 0, err
	}
	a.captureVideoMetadata(t.ID, safe)
	if platform == PlatformXiaohongshu && t.PublishedAt == nil {
		_, _ = a.RefreshTaskPublication(ctx, t.ID)
	}
	return safe, nil, info.Size(), nil
}
func (a *App) safeFile(path string) (string, error) {
	absolute, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(a.downloadDir)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", errors.New("文件超出下载目录")
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("文件不存在或不是普通文件")
	}
	return absolute, nil
}

// 到达启动间隔后，一次认领当前空位数量的任务，共用下一批的等待时间。
func (a *App) startNext(ctx context.Context) bool {
	type download struct {
		task   Task
		ctx    context.Context
		cancel context.CancelFunc
	}
	var batch []download
	var settings Settings
	err := a.transaction(func() error {
		now := time.Now()
		if a.state.Paused {
			return nil
		}
		settings = a.state.Settings
		for _, platform := range []PlatformID{PlatformBilibili, PlatformXiaohongshu} {
			policy := settings.DownloadPolicies[platform]
			if policy.Concurrency < 1 || futureTime(a.state.NextPlatformDownloadAt[platform], now) || future(a.platformBlockedUntilLocked(platform), now) {
				continue
			}
			active := 0
			for i := range a.state.Tasks {
				t := &a.state.Tasks[i]
				if taskPlatform(*t) == platform && t.Status == "downloading" {
					active++
				}
			}
			if active >= policy.Concurrency {
				continue
			}
			started := 0
			for i := range a.state.Tasks {
				if active+started >= policy.Concurrency {
					break
				}
				t := &a.state.Tasks[i]
				if t.Status != "queued" || taskPlatform(*t) != platform || a.running[t.ID] != nil {
					continue
				}
				t.Status = "downloading"
				t.Speed, t.Error = "", ""
				taskCtx, cancel := context.WithCancel(ctx)
				a.running[t.ID] = cancel
				batch = append(batch, download{task: *t, ctx: taskCtx, cancel: cancel})
				started++
			}
			if started > 0 {
				next := now.Add(jitter(time.Duration(policy.MinInterval)*time.Minute, time.Duration(policy.MaxInterval)*time.Minute))
				if a.state.NextPlatformDownloadAt == nil {
					a.state.NextPlatformDownloadAt = map[PlatformID]time.Time{}
				}
				a.state.NextPlatformDownloadAt[platform] = next
				if platform == PlatformBilibili {
					a.state.NextBilibiliDownloadAt = &next
				}
			}
		}
		a.refreshNextDownloadAtLocked()
		return nil
	})
	if err != nil {
		a.mu.Lock()
		for _, item := range batch {
			item.cancel()
			delete(a.running, item.task.ID)
		}
		a.mu.Unlock()
		log.Printf("认领下载批次失败: %v", err)
		return false
	}
	for _, item := range batch {
		go func() {
			defer item.cancel()
			var file string
			var files []string
			var size int64
			var err error
			if item.task.Kind == "image_post" {
				file, files, size, err = a.downloadDynamic(item.ctx, item.task)
			} else {
				file, files, size, err = a.downloadVideo(item.ctx, item.task, settings)
			}
			a.finishTask(ctx, item.task, file, files, size, err)
			a.mu.Lock()
			delete(a.running, item.task.ID)
			a.mu.Unlock()
		}()
	}
	return len(batch) > 0
}

func taskPlatform(task Task) PlatformID {
	if task.Platform != "" {
		return task.Platform
	}
	if isBilibiliTask(task) {
		return PlatformBilibili
	}
	return PlatformXiaohongshu
}

func futureTime(value time.Time, now time.Time) bool { return !value.IsZero() && now.Before(value) }

func (a *App) refreshNextDownloadAtLocked() {
	var earliest *time.Time
	for _, at := range a.state.NextPlatformDownloadAt {
		if earliest == nil || at.Before(*earliest) {
			copy := at
			earliest = &copy
		}
	}
	a.state.NextDownloadAt = earliest
}

// Keep the result in memory until it has been committed. A transient disk error
// must not leave a downloading task without an owner.
func (a *App) finishTask(ctx context.Context, task Task, file string, files []string, size int64, downloadErr error) {
	for {
		saveErr := a.transaction(func() error {
			if downloadErr != nil && ctx.Err() == nil {
				platform := task.Platform
				if platform == "" {
					if isBilibiliTask(task) {
						platform = PlatformBilibili
					} else {
						platform = PlatformXiaohongshu
					}
				}
				a.blockPlatformLocked(platform, downloadErr.Error(), time.Now())
			}
			for i := range a.state.Tasks {
				t := &a.state.Tasks[i]
				if t.ID != task.ID || t.Status != "downloading" {
					continue
				}
				t.Speed = ""
				t.ETA = ""
				if ctx.Err() != nil {
					t.Status = "queued"
					return nil
				}
				if downloadErr != nil {
					t.Status = "failed"
					t.Error = downloadErr.Error()
					a.event("error", "下载失败「"+t.Title+"」: "+downloadErr.Error())
				} else {
					now := time.Now()
					t.Status = "completed"
					t.Progress = 100
					t.File = file
					t.Files = files
					t.Bytes = size
					t.FinishedAt = &now
					a.event("success", "已归档「"+t.Title+"」")
				}
				return nil
			}
			return nil
		})
		if saveErr == nil {
			return
		}
		log.Printf("保存任务状态失败，稍后重试: %v", saveErr)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
func (a *App) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.mu.Lock()
			for _, cancel := range a.running {
				cancel()
			}
			a.mu.Unlock()
			return
		case <-ticker.C:
		}
		for a.startNext(ctx) {
		}
		state := a.Snapshot()
		if state.Paused {
			continue
		}
		a.startContentTotalsRefresh(ctx)
		var historyCreator *Creator
		for i := range state.Creators {
			c := &state.Creators[i]
			if !c.Enabled || c.HistoryJob == nil || c.HistoryJob.Status != "running" {
				continue
			}
			if historyCreator == nil || c.HistoryJob.NextPageAt.Before(historyCreator.HistoryJob.NextPageAt) {
				historyCreator = c
			}
		}
		if historyCreator != nil {
			go func(id string) {
				if err := a.ProcessHistoryPage(ctx, id); err != nil && ctx.Err() == nil {
					log.Printf("历史拉取: %v", err)
				}
			}(historyCreator.ID)
		}
		for _, c := range state.Creators {
			if !c.Enabled || (c.NextCheckAt != nil && time.Now().Before(*c.NextCheckAt)) || (c.LastChecked != nil && time.Since(*c.LastChecked) < time.Duration(c.Interval)*time.Minute) {
				continue
			}
			if future(platformCooldown(state, c.Platform), time.Now()) {
				continue
			}
			a.mu.Lock()
			busy := len(a.checking) >= 2 || a.checking[c.ID]
			a.mu.Unlock()
			if busy {
				continue
			}
			// Reservation is made synchronously by ScanCreator before the next scheduler tick.
			go func(id string) {
				if err := a.ScanCreator(ctx, id); err != nil && ctx.Err() == nil {
					log.Printf("订阅检查: %v", err)
				}
			}(c.ID)
			break
		}
	}
}

// Drain waits for child processes and in-flight scans during graceful shutdown.
func (a *App) Drain(ctx context.Context) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		a.mu.Lock()
		idle := len(a.running) == 0 && len(a.checking) == 0
		a.mu.Unlock()
		if idle {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

var _ io.Writer = (*cappedBuffer)(nil)
