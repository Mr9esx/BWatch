package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeDownloader(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	fixture := filepath.Join(dir, "playlist.json")
	binary := filepath.Join(dir, "yt-dlp")
	script := `#!/bin/sh
case "$*" in
 *--flat-playlist*) cat '` + fixture + `'; exit 0;;
esac
while [ "$#" -gt 0 ]; do
 if [ "$1" = "-P" ]; then shift; directory="$1"; fi
 shift
done
mkdir -p "$directory"
printf 'video' > "$directory/test.mp4"
printf 'BF_PROGRESS:50.0%%|1 MiB/s|00:01|3\n'
printf 'BF_FILE:%s/test.mp4\n' "$directory"
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return binary, fixture
}
func useTestCookies(t *testing.T, a *App) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(path, []byte("# Netscape HTTP Cookie File\n.bilibili.com\tTRUE\t/\tTRUE\t0\tbuvid3\tfixture-visitor\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a.state.Settings.CookiesFile = path
}
func TestScanToDownloadAndArchive(t *testing.T) {
	binary, fixture := fakeDownloader(t)
	a, err := New(t.TempDir(), t.TempDir(), binary)
	if err != nil {
		t.Fatal(err)
	}
	useTestCookies(t, a)
	c, err := a.AddCreator(CreatorInput{URL: "https://www.youtube.com/@fixture", Interval: 10, Quality: "1080"})
	if err != nil {
		t.Fatal(err)
	}
	write := func(data string) {
		t.Helper()
		if err := os.WriteFile(fixture, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"uploader":"UP","entries":[{"id":"BVold","title":"旧视频"}]}`)
	if err := a.ScanCreator(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	write(`{"uploader":"UP","entries":[{"id":"BVnew","title":"新视频"},{"id":"BVold","title":"旧视频"}]}`)
	if err := a.ScanCreator(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if !a.startNext(context.Background()) {
		t.Fatal("新视频没有开始下载")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	tasks := a.Snapshot().Tasks
	if len(tasks) != 1 || tasks[0].Status != "completed" || tasks[0].Bytes != 5 {
		t.Fatalf("归档失败: %+v", tasks)
	}
	contents, err := os.ReadFile(tasks[0].File)
	if err != nil || string(contents) != "video" {
		t.Fatalf("文件产物错误: %s %v", contents, err)
	}
}
func TestFailedScanNeverInitializesBaseline(t *testing.T) {
	binary, fixture := fakeDownloader(t)
	a, err := New(t.TempDir(), t.TempDir(), binary)
	if err != nil {
		t.Fatal(err)
	}
	useTestCookies(t, a)
	c, err := a.AddCreator(CreatorInput{URL: "https://www.youtube.com/@fixture", Interval: 10, Quality: "best"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture, []byte(`{"title":"not a playlist"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.ScanCreator(context.Background(), c.ID); err == nil {
		t.Fatal("缺少列表应返回错误")
	}
	got := a.Snapshot().Creators[0]
	if got.Initialized || !strings.Contains(got.LastError, "未返回") {
		t.Fatal("失败扫描不应建立基线，且应保留错误")
	}
}
func TestTextDynamicIsArchivedWithoutYtDlp(t *testing.T) {
	a := testApp(t)
	watch := false
	c, err := a.AddCreator(CreatorInput{URL: "123", Interval: 10, Quality: "best", WatchVideos: &watch, WatchDynamics: true, History: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ApplyDynamicScan(c.ID, []Entry{{ID: "123456", Title: "文字动态", Content: "你好，世界", URL: "https://t.bilibili.com/123456"}}, "UP"); err != nil {
		t.Fatal(err)
	}
	if !a.startNext(context.Background()) {
		t.Fatal("动态应开始归档")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	task := a.Snapshot().Tasks[0]
	if task.Status != "completed" {
		t.Fatalf("动态归档失败: %+v", task)
	}
	content, err := os.ReadFile(task.File)
	if err != nil || !strings.Contains(string(content), "你好，世界") {
		t.Fatal("Markdown 应保存动态正文")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(task.File), "metadata.json")); err != nil {
		t.Fatal("应保存元数据")
	}
}

func TestPauseStopsDownloaderChildren(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "yt-dlp")
	marker := filepath.Join(t.TempDir(), "child-started")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nsleep 20 &\nprintf '%s' \"$!\" > '"+marker+"'\nwait\n"), 0700); err != nil {
		t.Fatal(err)
	}
	a, err := New(t.TempDir(), t.TempDir(), binary)
	if err != nil {
		t.Fatal(err)
	}
	useTestCookies(t, a)
	c, err := a.AddCreator(CreatorInput{URL: "123", Interval: 10, Quality: "best", History: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ApplyScan(c.ID, []Entry{{ID: "video", URL: "https://www.bilibili.com/video/BVone"}}, ""); err != nil {
		t.Fatal(err)
	}
	if !a.startNext(context.Background()) {
		t.Fatal("没有开始任务")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("子进程未启动")
		}
		time.Sleep(10 * time.Millisecond)
	}
	id := a.Snapshot().Tasks[0].ID
	if err := a.TaskAction(id, "pause"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.Drain(ctx); err != nil {
		t.Fatal("暂停必须终止持有下载输出的子进程:", err)
	}
	if a.Snapshot().Tasks[0].Status != "paused" {
		t.Fatal("任务必须保持暂停")
	}
}

func TestFinishRetriesAfterTemporaryStoreFailure(t *testing.T) {
	a := testApp(t)
	task := Task{ID: "recoverable", Title: "已下载内容", Status: "downloading"}
	a.state.Tasks = []Task{task}
	if err := os.Remove(a.statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(a.statePath, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { a.finishTask(ctx, task, "archive.mp4", nil, 5, nil); close(done) }()
	select {
	case <-done:
		t.Fatal("落盘失败时不能放弃完成结果")
	case <-time.After(100 * time.Millisecond):
	}
	if err := os.Remove(a.statePath); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("写盘恢复后应完成提交")
	}
	state := a.Snapshot()
	if state.Tasks[0].Status != "completed" || state.Tasks[0].File != "archive.mp4" {
		t.Fatalf("完成结果丢失: %+v", state.Tasks[0])
	}
	reopened, err := New(filepath.Dir(a.statePath), a.downloadDir, a.binary)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Snapshot().Tasks[0].Status != "completed" {
		t.Fatal("恢复后的完成结果必须持久化")
	}
}
