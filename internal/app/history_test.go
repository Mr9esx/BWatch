package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistoryCanBeStartedForExistingBaseline(t *testing.T) {
	a := testApp(t)
	c, err := a.AddCreator(CreatorInput{URL: "123", Interval: 30, Quality: "best"})
	if err != nil {
		t.Fatal(err)
	}
	if err = a.ApplyScan(c.ID, []Entry{{ID: "BVold", Title: "旧投稿"}}, "UP"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/creators/"+c.ID+"/history", strings.NewReader(`{"limit":50,"watchVideos":true,"watchDynamics":false}`))
	a.Handler(context.Background(), "", "").ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatalf("应允许已有订阅回补历史，got %d: %s", w.Code, w.Body.String())
	}
}

func TestDefaultQualityIsHighestAvailable(t *testing.T) {
	if q := testApp(t).Snapshot().Settings.Quality; q != "best" {
		t.Fatalf("默认应选择最高可用画质，got %s", q)
	}
}

func TestHistoryPaginationDedupAndResumeSurviveRestart(t *testing.T) {
	a := testApp(t)
	useTestCookies(t, a)
	c, _ := a.AddCreator(CreatorInput{URL: "123", Interval: 30, Quality: "best"})
	if err := a.ApplyScan(c.ID, []Entry{{ID: "BVseen"}}, "UP"); err != nil {
		t.Fatal(err)
	}
	if err := a.StartHistory(c.ID, HistoryInput{Limit: 2, WatchVideos: true}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	transport := testTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		vid, offset, more := "BVseen", "next", true
		if r.URL.Query().Get("offset") == "next" {
			vid, offset, more = "BVsecond", "", false
		}
		body := fmt.Sprintf(`{"code":0,"data":{"has_more":%t,"offset":%q,"items":[{"id_str":"post","type":"DYNAMIC_TYPE_AV","modules":{"module_dynamic":{"major":{"archive":{"bvid":%q,"title":"历史视频"}}}}}]}}`, more, offset, vid)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	a.httpClient = &http.Client{Transport: transport}
	if err := a.ProcessHistoryPage(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	s := a.Snapshot()
	if len(s.Tasks) != 1 || s.Tasks[0].VideoID != "BVseen" || s.Tasks[0].Quality != "best" || s.Creators[0].HistoryJob.Offset != "next" {
		t.Fatalf("应回补已建基线但未下载的历史: %+v", s)
	}
	b, err := New(filepath.Dir(a.statePath), a.downloadDir, a.binary)
	if err != nil {
		t.Fatal(err)
	}
	b.httpClient = &http.Client{Transport: transport}
	if err = b.ProcessHistoryPage(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("重启不能绕过翻页间隔")
	}
	if err = b.HistoryAction(c.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	readyHistory(t, b, c.ID)
	if err = b.ProcessHistoryPage(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("暂停时不应拉取")
	}
	if err = b.HistoryAction(c.ID, "resume"); err != nil {
		t.Fatal(err)
	}
	readyHistory(t, b, c.ID)
	if err = b.ProcessHistoryPage(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	s = b.Snapshot()
	if calls != 2 || len(s.Tasks) != 2 || s.Tasks[1].VideoID != "BVsecond" || s.Creators[0].HistoryJob.Status != "completed" {
		t.Fatalf("应从已保存游标继续: calls=%d tasks=%+v job=%+v", calls, s.Tasks, s.Creators[0].HistoryJob)
	}
	if err = b.ApplyScan(c.ID, []Entry{{ID: "BVseen"}, {ID: "BVsecond"}}, "UP"); err != nil {
		t.Fatal(err)
	}
	if len(b.Snapshot().Tasks) != 2 {
		t.Fatal("历史回补后正常监听不能重复入队")
	}
}
func readyHistory(t *testing.T, a *App, cid string) {
	t.Helper()
	if err := a.transaction(func() error {
		a.state.NextHistoryPageAt = nil
		for i := range a.state.Creators {
			if a.state.Creators[i].ID == cid {
				a.state.Creators[i].HistoryJob.NextPageAt = time.Now().Add(-time.Second)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBilibiliPacingAndGlobalConcurrency(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "downloader")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nsleep 20\n"), 0700); err != nil {
		t.Fatal(err)
	}
	a, err := New(t.TempDir(), t.TempDir(), binary)
	if err != nil {
		t.Fatal(err)
	}
	useTestCookies(t, a)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		d, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		if err := a.Drain(d); err != nil {
			t.Error(err)
		}
	}()
	if err = a.transaction(func() error {
		a.state.Settings.Concurrency = 2
		a.state.Tasks = []Task{{ID: "b1", Kind: "video", URL: "https://www.bilibili.com/video/BV1", Status: "queued", Quality: "best"}, {ID: "b2", Kind: "video", URL: "https://www.bilibili.com/video/BV2", Status: "queued", Quality: "best"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	if !a.startNext(ctx) || a.startNext(ctx) {
		t.Fatal("随机间隔期间不能同时启动第二条 B 站任务")
	}
	s := a.Snapshot()
	if s.NextBilibiliDownloadAt == nil || s.NextBilibiliDownloadAt.Before(before.Add(time.Minute)) || s.NextBilibiliDownloadAt.After(time.Now().Add(3*time.Minute)) {
		t.Fatal("应持久化 1–3 分钟随机间隔")
	}
	if err = a.transaction(func() error {
		a.state.Tasks = append(a.state.Tasks, Task{ID: "y1", Kind: "video", URL: "https://www.youtube.com/watch?v=x", Status: "queued", Quality: "best"}, Task{ID: "y2", Kind: "video", URL: "https://www.youtube.com/watch?v=y", Status: "queued", Quality: "best"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !a.startNext(ctx) || a.startNext(ctx) {
		t.Fatal("B站间隔不阻塞其他平台，但必须遵守总并发上限")
	}
	b, err := New(filepath.Dir(a.statePath), a.downloadDir, binary)
	if err != nil {
		t.Fatal(err)
	}
	if !future(b.Snapshot().NextBilibiliDownloadAt, time.Now()) {
		t.Fatal("重启不能清空下载等待时间")
	}
}

func TestHistoryRiskCooldownBlocksEveryBilibiliPath(t *testing.T) {
	a := testApp(t)
	useTestCookies(t, a)
	c, _ := a.AddCreator(CreatorInput{URL: "123", Interval: 30, Quality: "best"})
	if err := a.StartHistory(c.ID, HistoryInput{WatchVideos: true}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 412, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("risk"))}, nil
	})}
	if err := a.ProcessHistoryPage(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	s := a.Snapshot()
	if !future(s.BilibiliBlockedUntil, time.Now().Add(29*time.Minute)) || s.Creators[0].HistoryJob.Offset != "" {
		t.Fatal("412 应全局冷却至少 30 分钟且保留游标")
	}
	readyHistory(t, a, c.ID)
	if err := a.ProcessHistoryPage(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.ScanCreator(context.Background(), c.ID); err == nil {
		t.Fatal("手动检查也必须尊重风控冷却")
	}
	if calls != 1 {
		t.Fatal("冷却期间不应再次访问 B站")
	}
	if err := a.transaction(func() error {
		a.state.Tasks = []Task{{ID: "blocked", URL: "https://t.bilibili.com/123", Kind: "dynamic", Status: "queued"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if a.startNext(context.Background()) {
		t.Fatal("冷却期间不能启动B站下载")
	}
	b, err := New(filepath.Dir(a.statePath), a.downloadDir, a.binary)
	if err != nil {
		t.Fatal(err)
	}
	if !future(b.Snapshot().BilibiliBlockedUntil, time.Now()) {
		t.Fatal("冷却状态必须在重启后保留")
	}
}

func TestHistoryBackpressureKeepsPartialPageAndCountsConcurrentQueue(t *testing.T) {
	a := testApp(t)
	useTestCookies(t, a)
	c, _ := a.AddCreator(CreatorInput{URL: "123", Interval: 30, Quality: "best"})
	if err := a.StartHistory(c.ID, HistoryInput{WatchVideos: true}); err != nil {
		t.Fatal(err)
	}
	if err := a.transaction(func() error {
		for i := 0; i < 49; i++ {
			a.state.Tasks = append(a.state.Tasks, Task{ID: fmt.Sprint(i), URL: "https://t.bilibili.com/1", Kind: "dynamic", Status: "paused"})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			if err := a.transaction(func() error {
				a.state.Tasks = append(a.state.Tasks, Task{ID: "concurrent", URL: "https://t.bilibili.com/2", Kind: "dynamic", Status: "queued"})
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
		body := `{"code":0,"data":{"has_more":true,"offset":"next","items":[{"id_str":"p1","type":"DYNAMIC_TYPE_AV","modules":{"module_dynamic":{"major":{"archive":{"bvid":"BVfirst"}}}}},{"id_str":"p2","type":"DYNAMIC_TYPE_AV","modules":{"module_dynamic":{"major":{"archive":{"bvid":"BVsecond"}}}}}]}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	for round := 0; round < 3; round++ {
		if round > 0 {
			if err := a.TaskAction(fmt.Sprint(round-1), "delete"); err != nil {
				t.Fatal(err)
			}
			readyHistory(t, a, c.ID)
		}
		if err := a.ProcessHistoryPage(context.Background(), c.ID); err != nil {
			t.Fatal(err)
		}
		s := a.Snapshot()
		if len(s.Tasks) != 50 || s.Creators[0].HistoryJob.Videos != round {
			t.Fatalf("并发入队后仍需将积压限制为50，round=%d tasks=%d videos=%d", round, len(s.Tasks), s.Creators[0].HistoryJob.Videos)
		}
		if round < 2 && s.Creators[0].HistoryJob.Offset != "" {
			t.Fatal("半页入队时不能推进游标，否则会漏内容")
		}
		if round == 2 && s.Creators[0].HistoryJob.Offset != "next" {
			t.Fatal("处理完整页后应推进游标")
		}
	}
}

func TestHistoryStopsOnCyclicCursor(t *testing.T) {
	a := testApp(t)
	useTestCookies(t, a)
	c, _ := a.AddCreator(CreatorInput{URL: "123", Interval: 30, Quality: "best"})
	_ = a.StartHistory(c.ID, HistoryInput{WatchVideos: true})
	calls := 0
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		offset := "A"
		if calls == 2 {
			offset = "B"
		}
		body := fmt.Sprintf(`{"code":0,"data":{"has_more":true,"offset":%q,"items":[]}}`, offset)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	for i := 0; i < 3; i++ {
		readyHistory(t, a, c.ID)
		if err := a.ProcessHistoryPage(context.Background(), c.ID); err != nil {
			t.Fatal(err)
		}
	}
	if a.Snapshot().Creators[0].HistoryJob.Status != "failed" {
		t.Fatal("分页游标循环时必须停止，不能无限请求平台")
	}
}

func TestHistoryStillCoolsDownIfPausedDuringRiskResponse(t *testing.T) {
	a := testApp(t)
	useTestCookies(t, a)
	c, _ := a.AddCreator(CreatorInput{URL: "123", Interval: 30, Quality: "best"})
	_ = a.StartHistory(c.ID, HistoryInput{WatchVideos: true})
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		if err := a.HistoryAction(c.ID, "pause"); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 429, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("limited"))}, nil
	})}
	if err := a.ProcessHistoryPage(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	s := a.Snapshot()
	if !future(s.BilibiliBlockedUntil, time.Now()) || s.Creators[0].HistoryJob.Status != "paused" {
		t.Fatal("即使历史任务被暂停，已经观察到的风控也必须全局生效")
	}
}

func TestHistoryCanQueueVideosAndImagePostsTogether(t *testing.T) {
	a := testApp(t)
	useTestCookies(t, a)
	c, _ := a.AddCreator(CreatorInput{URL: "123", Interval: 30, Quality: "best", WatchDynamics: true})
	if err := a.StartHistory(c.ID, HistoryInput{Limit: 2, WatchVideos: true, WatchDynamics: true}); err != nil {
		t.Fatal(err)
	}
	body := `{"code":0,"data":{"has_more":false,"items":[{"id_str":"v","type":"DYNAMIC_TYPE_AV","modules":{"module_dynamic":{"major":{"archive":{"bvid":"BVmixed","title":"最高画质视频"}}}}},{"id_str":"image-post","type":"DYNAMIC_TYPE_DRAW","modules":{"module_dynamic":{"desc":{"text":"图文正文"},"major":{"draw":{"items":[{"src":"https://i0.hdslb.com/original.jpg"}]}}}}}]}}`
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	if err := a.ProcessHistoryPage(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	s := a.Snapshot()
	h := s.Creators[0].HistoryJob
	if len(s.Tasks) != 2 || h.Status != "completed" || h.Videos != 1 || h.Dynamics != 1 || s.Tasks[1].Kind != "dynamic" || s.Tasks[1].Content != "图文正文" || len(s.Tasks[1].Images) != 1 || s.Tasks[0].Quality != "best" {
		t.Fatalf("视频与图文应共同回补且保留原图正文: job=%+v tasks=%+v", h, s.Tasks)
	}
}
