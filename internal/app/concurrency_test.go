package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDownloadBatchStartsTogetherAndWaitsBetweenBatches(t *testing.T) {
	a := testApp(t)
	defer a.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		drain, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		if err := a.Drain(drain); err != nil {
			t.Error(err)
		}
	}()
	started := make(chan string, 8)
	finishFirst := make(chan struct{})
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		started <- r.URL.Path
		var finish <-chan struct{}
		if r.URL.Path == "/0.jpg" {
			finish = finishFirst
		}
		select {
		case <-r.Context().Done():
			return nil, r.Context().Err()
		case <-finish:
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("image bytes"))}, nil
	})}
	if err := a.transaction(func() error {
		a.state.Settings.Concurrency = 4
		a.state.Settings.DownloadMinInterval, a.state.Settings.DownloadMaxInterval = 1, 2
		future := time.Now().Add(time.Minute)
		a.state.NextDownloadAt, a.state.NextBilibiliDownloadAt = &future, &future
		for i := 0; i < 6; i++ {
			a.state.Tasks = append(a.state.Tasks, Task{ID: fmt.Sprint(i), CreatorID: "fixture", Platform: PlatformBilibili, Kind: ContentImagePost, Status: "queued", Images: []string{fmt.Sprintf("https://i0.hdslb.com/%d.jpg", i)}})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if a.startNext(ctx) {
		t.Fatal("间隔未到时不能启动下一批")
	}
	expireBatch := func() {
		t.Helper()
		if err := a.transaction(func() error { past := time.Now().Add(-time.Second); a.state.NextDownloadAt = &past; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	expireBatch()
	before := time.Now()
	if !a.startNext(ctx) {
		t.Fatal("间隔到达后应启动一批任务")
	}
	if a.startNext(ctx) {
		t.Fatal("不能超过4个并发任务")
	}
	for i := 0; i < 4; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("任务未实际开始下载")
		}
	}
	if s := a.Snapshot(); s.NextDownloadAt == nil || s.NextDownloadAt.Before(before.Add(time.Minute)) || s.NextDownloadAt.After(time.Now().Add(2*time.Minute)) || s.NextBilibiliDownloadAt == nil || !s.NextDownloadAt.Equal(*s.NextBilibiliDownloadAt) {
		t.Fatal("整批启动后应持久化下一批的随机等待时间")
	}
	close(finishFirst)
	deadline := time.Now().Add(3 * time.Second)
	for {
		a.mu.Lock()
		remaining := len(a.running)
		a.mu.Unlock()
		if remaining == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("首个任务没有完成并释放槽位")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if a.startNext(ctx) {
		t.Fatal("任务完成释放空位后，仍须等待下一批间隔")
	}
	expireBatch()
	if !a.startNext(ctx) {
		t.Fatal("下一批到点时应填入空位")
	}
	select {
	case path := <-started:
		if path != "/4.jpg" {
			t.Fatal("补入了错误的任务", path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("补位任务未实际下载")
	}
	if a.startNext(ctx) {
		t.Fatal("补位后仍应遵守4个任务上限")
	}
}

func TestDownloadBatchKeepsSerialSpacingAndPlatformCooldown(t *testing.T) {
	a := testApp(t)
	defer a.Close()
	future := time.Now().Add(time.Minute)
	a.state.Tasks = []Task{{ID: "pending", Platform: PlatformBilibili, Kind: ContentImagePost, Status: "queued"}}
	a.state.Settings.Concurrency = 1
	a.state.NextDownloadAt = &future
	if a.startNext(context.Background()) {
		t.Fatal("单任务模式仍应遵守启动间隔")
	}
	a.state.Settings.Concurrency = 4
	a.state.NextDownloadAt = nil
	a.state.PlatformCooldowns = map[PlatformID]time.Time{PlatformBilibili: future}
	if a.startNext(context.Background()) {
		t.Fatal("批量并发不能绕过平台风控冷却")
	}
	a.state.PlatformCooldowns = nil
	a.state.Paused = true
	if a.startNext(context.Background()) {
		t.Fatal("暂停调度时不能启动任务")
	}
}

func TestDownloadBatchClaimRollsBackAllOwners(t *testing.T) {
	a := testApp(t)
	a.state.Settings.Concurrency = 4
	a.state.Tasks = []Task{{ID: "one", Platform: PlatformBilibili, Kind: ContentImagePost, Status: "queued"}, {ID: "two", Platform: PlatformBilibili, Kind: ContentImagePost, Status: "queued"}}
	a.Close()
	if a.startNext(context.Background()) {
		t.Fatal("状态保存失败不能启动任务")
	}
	if len(a.running) != 0 {
		t.Fatal("失败批次不能留下虚假的运行任务")
	}
	for _, task := range a.Snapshot().Tasks {
		if task.Status != "queued" {
			t.Fatal("失败批次必须恢复排队状态")
		}
	}
	if a.Snapshot().NextDownloadAt != nil {
		t.Fatal("失败批次不能消耗下一批间隔")
	}
}
