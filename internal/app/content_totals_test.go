package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func remoteContentTotals(t *testing.T, c Creator) (*int, *int) {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Totals *struct {
			Videos   *int `json:"videos"`
			Dynamics *int `json:"dynamics"`
		} `json:"contentTotals"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Totals == nil {
		return nil, nil
	}
	return wire.Totals.Videos, wire.Totals.Dynamics
}

func TestSchedulerBackfillsMissingTotalsBeforeNextContentCheck(t *testing.T) {
	a := testApp(t)
	useTestCookies(t, a)
	cached, _ := a.AddCreator(CreatorInput{URL: "26800964", Interval: 1440, Quality: "best"})
	missing, _ := a.AddCreator(CreatorInput{URL: "3546898497538707", Interval: 1440, Quality: "best"})
	now, known := time.Now(), 393
	if err := a.transaction(func() error {
		for i := range a.state.Creators {
			a.state.Creators[i].LastChecked = &now
		}
		a.state.Creators[0].ContentTotals = &ContentTotals{Videos: &known, CheckedAt: now}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Path != "/x/space/navnum" || r.URL.Query().Get("mid") != "3546898497538707" {
			t.Errorf("只应补齐缺失总量，不能重新扫描内容或刷新未过期缓存: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"video":100,"opus":0}}`))}, nil
	})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); a.Run(ctx) }()
	defer func() {
		cancel()
		<-done
		drainCtx, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		if err := a.Drain(drainCtx); err != nil {
			t.Error(err)
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s := a.Snapshot()
		video, dynamic := remoteContentTotals(t, s.Creators[1])
		if video != nil && dynamic != nil {
			if *video != 100 || *dynamic != 0 || calls.Load() != 1 || len(s.Tasks) != 0 || s.Creators[0].ID != cached.ID || s.Creators[1].ID != missing.ID || !s.Creators[1].LastChecked.Equal(now) {
				t.Fatal("补齐总量不能触发内容检查、下载，或重复刷新已有缓存")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("刚检查过的订阅仍缺少总量：后台必须独立补齐，不能等下一轮内容检查")
}

func TestTotalsBackfillRespectsPauseCooldownAndInFlightCheck(t *testing.T) {
	a := testApp(t)
	useTestCookies(t, a)
	c, _ := a.AddCreator(CreatorInput{URL: "123", Interval: 30, Quality: "best"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := a.SetPaused(true); err != nil {
		t.Fatal(err)
	}
	if a.startContentTotalsRefresh(ctx) {
		t.Fatal("全局暂停时不能请求总量")
	}
	if err := a.SetPaused(false); err != nil {
		t.Fatal(err)
	}
	if err := a.transaction(func() error { at := time.Now().Add(time.Hour); a.state.BilibiliBlockedUntil = &at; return nil }); err != nil {
		t.Fatal(err)
	}
	if a.startContentTotalsRefresh(ctx) {
		t.Fatal("风控冷却时不能补齐总量")
	}
	if err := a.transaction(func() error { a.state.BilibiliBlockedUntil = nil; a.state.Creators[0].Enabled = false; return nil }); err != nil {
		t.Fatal(err)
	}
	if a.startContentTotalsRefresh(ctx) {
		t.Fatal("暂停的订阅不能自动请求总量")
	}
	if err := a.transaction(func() error { a.state.Creators[0].Enabled = true; return nil }); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"video":100,"opus":0}}`))}, nil
	})}
	defer func() {
		cancel()
		d, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		if err := a.Drain(d); err != nil {
			t.Error(err)
		}
	}()
	if !a.startContentTotalsRefresh(ctx) {
		t.Fatal("启用后应自动补齐")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("总量请求没有启动")
	}
	if a.startContentTotalsRefresh(ctx) {
		t.Fatal("请求未完成时不能重复调度同一个 UP")
	}
	if err := a.ScanCreator(ctx, c.ID); err == nil {
		t.Fatal("内容检查不能与同一 UP 的统计请求重叠")
	}
	close(release)
	d, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := a.Drain(d); err != nil {
		t.Fatal(err)
	}
	if a.startContentTotalsRefresh(ctx) || calls.Load() != 1 {
		t.Fatal("成功后应复用缓存，不能循环请求")
	}
}

func TestPlatformContentTotalsAreFetchedOnceAndPersistedWithoutDownloading(t *testing.T) {
	a := testApp(t)
	useTestCookies(t, a)
	c, _ := a.AddCreator(CreatorInput{URL: "26800964", Interval: 30, Quality: "best"})
	feedCalls, totalCalls := 0, 0
	transport := testTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"code":0,"data":{"has_more":true,"offset":"","items":[]}}`
		if r.URL.Path == "/x/space/navnum" {
			totalCalls++
			if r.URL.Query().Get("mid") != "26800964" || r.Header.Get("Cookie") == "" || r.Header.Get("Referer") == "" {
				t.Fatal("总量请求应使用目标 UID 和现有登录会话")
			}
			body = `{"code":0,"data":{"video":393,"opus":44,"article":0}}`
		} else {
			feedCalls++
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	a.httpClient = &http.Client{Transport: transport}
	for i := 0; i < 2; i++ {
		if err := a.ScanCreator(context.Background(), c.ID); err != nil {
			t.Fatal(err)
		}
	}
	s := a.Snapshot()
	videos, dynamics := remoteContentTotals(t, s.Creators[0])
	if videos == nil || *videos != 393 || dynamics == nil || *dynamics != 44 || totalCalls != 1 || feedCalls != 2 || len(s.Tasks) != 0 {
		t.Fatalf("不完整列表也应直接拿到平台总量，缓存避免重复查询且不下载: video=%v dynamic=%v totalsCalls=%d feedCalls=%d tasks=%d", videos, dynamics, totalCalls, feedCalls, len(s.Tasks))
	}
	b, err := New(filepath.Dir(a.statePath), a.downloadDir, a.binary)
	if err != nil {
		t.Fatal(err)
	}
	b.httpClient = &http.Client{Transport: transport}
	if err = b.ScanCreator(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if totalCalls != 1 {
		t.Fatal("重启也应复用总量缓存")
	}
}

func TestPlatformTotalsDistinguishZeroMissingErrorsAndRisk(t *testing.T) {
	for _, tc := range []struct {
		name, body     string
		status         int
		video, dynamic bool
		risk           bool
	}{
		{"零总数", `{"code":0,"data":{"video":0,"opus":0}}`, 200, true, true, false},
		{"图文字段缺失", `{"code":0,"data":{"video":12,"article":8}}`, 200, true, false, false},
		{"字段全部缺失", `{"code":0,"data":{}}`, 200, false, false, false},
		{"负数不能作为总量", `{"code":0,"data":{"video":-1,"opus":2}}`, 200, false, false, false},
		{"业务错误", `{"code":-101,"data":{"video":12,"opus":8}}`, 200, false, false, false},
		{"风控错误", `{"code":-352}`, 200, false, false, true},
		{"限流", `{}`, 429, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t)
			useTestCookies(t, a)
			c, _ := a.AddCreator(CreatorInput{URL: "123", Interval: 30, Quality: "best"})
			a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				status, body := 200, `{"code":0,"data":{"has_more":false,"items":[]}}`
				if r.URL.Path == "/x/space/navnum" {
					status, body = tc.status, tc.body
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			if err := a.ScanCreator(context.Background(), c.ID); err != nil {
				t.Fatal("总量查询失败不能破坏已成功的内容监听", err)
			}
			s := a.Snapshot()
			video, dynamic := remoteContentTotals(t, s.Creators[0])
			if (video != nil) != tc.video || (dynamic != nil) != tc.dynamic || (s.BilibiliBlockedUntil != nil) != tc.risk {
				t.Fatalf("总量可用性/冷却不正确: video=%v dynamic=%v blocked=%v", video, dynamic, s.BilibiliBlockedUntil)
			}
		})
	}
}
