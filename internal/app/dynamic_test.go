package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDynamicDownloadsImagesAndUpgradesKnownHTTPHosts(t *testing.T) {
	a := testApp(t)
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "i0.hdslb.com" {
			t.Errorf("图片应从受信任 CDN 的 HTTPS 地址下载: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/jpeg"}}, Body: io.NopCloser(strings.NewReader("original image"))}, nil
	})}
	file, images, size, err := a.downloadDynamic(context.Background(), Task{ID: "image-test", CreatorID: "up", Title: "图文", Content: "配文", Images: []string{"http://i0.hdslb.com/image.jpg"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || size <= 14 {
		t.Fatal("应保存正文与原图")
	}
	image, err := os.ReadFile(images[0])
	if err != nil || string(image) != "original image" {
		t.Fatal("没有保存原图")
	}
	markdown, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(markdown), "image-01.jpg") {
		t.Fatal("Markdown 应引用归档后的图片")
	}
}

func TestDynamicDrawOpusAndForwardParsing(t *testing.T) {
	raw := `{"code":0,"data":{"items":[{"id_str":"1","type":"DYNAMIC_TYPE_DRAW","modules":{"module_author":{"name":"UP"},"module_dynamic":{"desc":{"text":"今天的照片"},"major":{"draw":{"items":[{"src":"https://i0.hdslb.com/a.jpg"}]}}}}},{"id_str":"2","type":"DYNAMIC_TYPE_WORD","modules":{"module_dynamic":{"major":{"opus":{"title":"游记","summary":{"text":"海边的一天"},"pics":[{"url":"https://i1.hdslb.com/b.png"}]}}}}},{"id_str":"3","type":"DYNAMIC_TYPE_AV","modules":{"module_dynamic":{"major":{"archive":{"bvid":"BV1"}}}}},{"id_str":"4","type":"DYNAMIC_TYPE_FORWARD","modules":{"module_dynamic":{"desc":{"text":"转发啦"}}},"orig":{"modules":{"module_dynamic":{"desc":{"text":"原文"},"major":{"draw":{"items":[{"src":"https://i0.hdslb.com/c.jpg"}]}}}}}}]}}`
	entries, name, _, _, err := parseDynamics(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if name != "UP" || len(entries) != 3 {
		t.Fatalf("应过滤视频动态，保留图文: %+v %q", entries, name)
	}
	if entries[0].Content != "今天的照片" || len(entries[0].Images) != 1 {
		t.Fatal("图片动态解析失败")
	}
	if entries[1].Title != "游记" || entries[1].Content != "海边的一天" {
		t.Fatal("opus 解析失败")
	}
	if !strings.Contains(entries[2].Content, "原文") || len(entries[2].Images) != 1 {
		t.Fatal("转发应保存原文和图片")
	}
}
func TestDynamicBaselineIndependentFromVideo(t *testing.T) {
	a := testApp(t)
	c, err := a.AddCreator(CreatorInput{URL: "123", Interval: 5, Quality: "best", WatchDynamics: true})
	if err != nil {
		t.Fatal(err)
	}
	entries := []Entry{{ID: "old", Title: "旧动态", Content: "旧"}}
	if err := a.ApplyDynamicScan(c.ID, entries, ""); err != nil {
		t.Fatal(err)
	}
	entries = append([]Entry{{ID: "new", Title: "新动态", Content: "新"}}, entries...)
	if err := a.ApplyDynamicScan(c.ID, entries, ""); err != nil {
		t.Fatal(err)
	}
	if err := a.ApplyScan(c.ID, []Entry{{ID: "video", Title: "旧视频"}}, ""); err != nil {
		t.Fatal(err)
	}
	tasks := a.Snapshot().Tasks
	if len(tasks) != 1 || tasks[0].Kind != "dynamic" || tasks[0].Content != "新" {
		t.Fatalf("独立基线失败: %+v", tasks)
	}
}
func TestDynamicAPIErrorsAreVisible(t *testing.T) {
	_, _, _, _, err := parseDynamics(strings.NewReader(`{"code":-352,"message":"风控校验失败"}`))
	if err == nil || !strings.Contains(err.Error(), "-352") {
		t.Fatal("必须保留平台错误码")
	}
}

func TestDesktopDynamicModulesPreserveTextImagesAndForward(t *testing.T) {
	raw := `{"code":0,"data":{"has_more":false,"items":[{"id_str":"101","type":"DYNAMIC_TYPE_FORWARD","modules":[{"module_type":"MODULE_TYPE_AUTHOR","module_author":{"user":{"name":"测试创作者"}}},{"module_type":"MODULE_TYPE_DESC","module_desc":{"text":"转发文字"}},{"module_type":"MODULE_TYPE_DYNAMIC","module_dynamic":{"type":"MDL_DYN_TYPE_FORWARD","dyn_forward":{"item":{"id_str":"100","type":"DYNAMIC_TYPE_DRAW","modules":[{"module_desc":{"text":"原始配文"}},{"module_dynamic":{"dyn_draw":{"items":[{"src":"http://i0.hdslb.com/original.jpg"}]}}}]}}}}]},{"id_str":"102","type":"DYNAMIC_TYPE_AV","modules":[{"module_author":{"user":{"name":"测试创作者"}}}]}]}}`
	entries, name, _, _, err := parseDynamics(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if name != "测试创作者" || len(entries) != 1 || !strings.Contains(entries[0].Content, "原始配文") || len(entries[0].Images) != 1 {
		t.Fatalf("桌面端结构应完整归一化，且过滤原创视频动态: %+v %q", entries, name)
	}
}

func TestBilibiliUsesOneFeedForVideoAndImageMonitoring(t *testing.T) {
	a := testApp(t)
	useTestCookies(t, a)
	calls := 0
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/x/space/navnum" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"video":1,"opus":1}}`))}, nil
		}
		calls++
		if r.URL.Path != "/x/polymer/web-dynamic/desktop/v1/feed/space" {
			t.Fatalf("不应再调用不稳定的视频 WBI 列表: %s", r.URL.Path)
		}
		body := `{"code":0,"data":{"has_more":false,"items":[{"id_str":"post-video","type":"DYNAMIC_TYPE_AV","modules":[{"module_author":{"user":{"name":"UP"}}},{"module_dynamic":{"dyn_archive":{"bvid":"BVtest123","title":"真实投稿标题","cover":"http://i0.hdslb.com/cover.jpg","duration_text":"01:23"}}}]},{"id_str":"post-image","type":"DYNAMIC_TYPE_DRAW","modules":[{"module_desc":{"text":"配文"}},{"module_dynamic":{"dyn_draw":{"items":[{"src":"https://i0.hdslb.com/image.jpg"}]}}}]}]}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	c, err := a.AddCreator(CreatorInput{URL: "123", Interval: 10, Quality: "480", History: true, WatchDynamics: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ScanCreator(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	s := a.Snapshot()
	if len(s.Tasks) != 2 || s.Tasks[0].Kind != "video" || s.Tasks[0].VideoID != "BVtest123" || s.Tasks[0].Duration != 83 || s.Tasks[0].Title != "真实投稿标题" || s.Tasks[1].Kind != "dynamic" {
		t.Fatalf("应一次发现视频与图文: %+v", s.Tasks)
	}
	if calls != 1 {
		t.Fatalf("同一轮应只请求一次动态页，实际 %d", calls)
	}
	if err := a.ScanCreator(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if len(a.Snapshot().Tasks) != 2 {
		t.Fatal("再次真实格式检查不应重复入队")
	}
}

func TestHistoryFollowsMixedFeedUntilSelectedTypesAreComplete(t *testing.T) {
	for _, exhausted := range []bool{false, true} {
		t.Run(fmt.Sprint("limit=", exhausted), func(t *testing.T) {
			a := testApp(t)
			useTestCookies(t, a)
			calls := 0
			a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/x/space/navnum" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"video":10,"opus":1}}`))}, nil
				}
				calls++
				items := []string{`{"id_str":"same-image","type":"DYNAMIC_TYPE_WORD","modules":{"module_dynamic":{"desc":{"text":"重复置顶动态"}}}}`}
				if !exhausted && calls == 4 {
					for i := 0; i < 10; i++ {
						items = append(items, fmt.Sprintf(`{"id_str":"video-%d","type":"DYNAMIC_TYPE_AV","modules":{"module_dynamic":{"major":{"archive":{"bvid":"BV%d","title":"视频"}}}}}`, i, i))
					}
				}
				body := fmt.Sprintf(`{"code":0,"data":{"has_more":%t,"offset":"%d","items":[%s]}}`, exhausted || calls < 4, calls, strings.Join(items, ","))
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			c, err := a.AddCreator(CreatorInput{URL: "123", Interval: 10, Quality: "480", History: true, WatchDynamics: true})
			if err != nil {
				t.Fatal(err)
			}
			err = a.ScanCreator(context.Background(), c.ID)
			s := a.Snapshot()
			if exhausted {
				if err == nil || !strings.Contains(err.Error(), "历史归档未取全") || s.Creators[0].Initialized || s.Creators[0].DynamicInitialized || len(s.Tasks) != 0 {
					t.Fatalf("触及上限必须明确报错且不建立不完整基线: err=%v creator=%+v", err, s.Creators[0])
				}
			} else if err != nil || calls != 4 || len(s.Tasks) != 11 {
				t.Fatalf("应翻到第 4 页获取视频，重复动态只入队一次: calls=%d tasks=%d err=%v", calls, len(s.Tasks), err)
			}
		})
	}
}
