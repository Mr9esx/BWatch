package app

import (
	"os"
	"path/filepath"
	"testing"
)

func testApp(t *testing.T) *App {
	t.Helper()
	a, err := New(t.TempDir(), t.TempDir(), "yt-dlp")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestBaselineThenOnlyNewVideos(t *testing.T) {
	a := testApp(t)
	c, err := a.AddCreator(CreatorInput{Name: "测试 UP", URL: "12345", Interval: 10, Quality: "1080"})
	if err != nil {
		t.Fatal(err)
	}
	old := []Entry{{ID: "BVold", Title: "旧视频", URL: "https://www.bilibili.com/video/BVold"}}
	if err := a.ApplyScan(c.ID, old, "测试 UP"); err != nil {
		t.Fatal(err)
	}
	if len(a.Snapshot().Tasks) != 0 {
		t.Fatal("首次基线不能下载历史")
	}
	entries := append([]Entry{{ID: "BVnew", Title: "新视频", URL: "https://www.bilibili.com/video/BVnew"}}, old...)
	for i := 0; i < 2; i++ {
		if err := a.ApplyScan(c.ID, entries, "测试 UP"); err != nil {
			t.Fatal(err)
		}
	}
	s := a.Snapshot()
	if len(s.Tasks) != 1 || s.Tasks[0].VideoID != "BVnew" || s.Tasks[0].Status != "queued" {
		t.Fatalf("新视频必须只入队一次: %+v", s.Tasks)
	}
}

func TestHistoryAndTaskTransitionsPersist(t *testing.T) {
	dir := t.TempDir()
	a, err := New(dir, t.TempDir(), "yt-dlp")
	if err != nil {
		t.Fatal(err)
	}
	c, err := a.AddCreator(CreatorInput{URL: "12345", Interval: 5, Quality: "best", History: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ApplyScan(c.ID, []Entry{{ID: "one", URL: "https://www.bilibili.com/video/BVone"}}, "UP"); err != nil {
		t.Fatal(err)
	}
	id := a.Snapshot().Tasks[0].ID
	if err := a.TaskAction(id, "pause"); err != nil {
		t.Fatal(err)
	}
	b, err := New(dir, t.TempDir(), "yt-dlp")
	if err != nil {
		t.Fatal(err)
	}
	if b.Snapshot().Tasks[0].Status != "paused" {
		t.Fatal("暂停状态应持久化")
	}
	if err := b.TaskAction(id, "resume"); err != nil {
		t.Fatal(err)
	}
	if b.Snapshot().Tasks[0].Status != "queued" {
		t.Fatal("恢复后应重新排队")
	}
	if err := b.TaskAction(id, "retry"); err == nil {
		t.Fatal("队列中任务不能重试")
	}
	if err := b.TaskAction("missing", "pause"); err == nil {
		t.Fatal("不存在的任务应报错")
	}
}

func TestCreatorURLValidation(t *testing.T) {
	cases := []struct {
		in, want string
		valid    bool
	}{
		{"12345", "https://space.bilibili.com/12345/video", true},
		{"https://space.bilibili.com/12345?spm_id_from=333", "https://space.bilibili.com/12345/video", true},
		{"https://www.youtube.com/@veritasium", "https://www.youtube.com/@veritasium/videos", true},
		{"http://localhost:8080", "", false}, {"https://space.bilibili.com.evil.test/123", "", false},
		{"https://www.youtube.com/watch?v=abc", "", false}, {"--exec touch /tmp/bad", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := NormalizeCreatorURL(tc.in)
			if (err == nil) != tc.valid || got != tc.want {
				t.Fatalf("got %q %v", got, err)
			}
		})
	}
}

func TestFailedSaveRollsBack(t *testing.T) {
	a := testApp(t)
	if err := os.Remove(a.statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(a.statePath, 0700); err != nil {
		t.Fatal(err)
	}
	_, err := a.AddCreator(CreatorInput{URL: "123", Interval: 10, Quality: "best"})
	if err == nil {
		t.Fatal("写入失败必须报告")
	}
	if len(a.Snapshot().Creators) != 0 {
		t.Fatal("写入失败应回滚内存")
	}
	if _, err := os.Stat(filepath.Dir(a.statePath)); err != nil {
		t.Fatal(err)
	}
}

func TestDuplicateAndInvalidSettings(t *testing.T) {
	a := testApp(t)
	input := CreatorInput{URL: "123", Interval: 10, Quality: "best"}
	if _, err := a.AddCreator(input); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddCreator(input); err == nil {
		t.Fatal("不能重复订阅")
	}
	settings := a.Snapshot().Settings
	settings.Concurrency = 0
	if err := a.UpdateSettings(settings); err == nil {
		t.Fatal("并发数不能为零")
	}
}
