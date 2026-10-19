package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func contentCatalog(t *testing.T, c Creator) (map[string]bool, bool) {
	t.Helper()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Catalog  map[string]bool `json:"contentCatalog"`
		Complete bool            `json:"contentCatalogComplete"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	return wire.Catalog, wire.Complete
}

func TestContentCatalogCountsUnselectedKindsAndOnlyCompletesAtEnd(t *testing.T) {
	a := testApp(t)
	useTestCookies(t, a)
	c, _ := a.AddCreator(CreatorInput{URL: "123", Interval: 30, Quality: "best"})
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		n, more, offset := 1, true, "next"
		if r.URL.Query().Get("offset") == "next" {
			n, more, offset = 2, false, ""
		}
		body := fmt.Sprintf(`{"code":0,"data":{"has_more":%t,"offset":%q,"items":[{"id_str":"post%d","type":"DYNAMIC_TYPE_AV","modules":{"module_dynamic":{"major":{"archive":{"bvid":"BV%d"}}}}},{"id_str":"draw%d","type":"DYNAMIC_TYPE_WORD","modules":{"module_dynamic":{"desc":{"text":"图文"}}}},{"id_str":"draw1","type":"DYNAMIC_TYPE_WORD","modules":{"module_dynamic":{"desc":{"text":"重复图文"}}}}]}}`, more, offset, n, n, n)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	if err := a.StartHistory(c.ID, HistoryInput{Limit: 1, WatchVideos: true}); err != nil {
		t.Fatal(err)
	}
	if err := a.ProcessHistoryPage(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	s := a.Snapshot()
	catalog, complete := contentCatalog(t, s.Creators[0])
	if len(catalog) != 2 || !catalog["video:BV1"] || !catalog["dynamic:draw1"] || complete || len(s.Tasks) != 1 {
		t.Fatalf("限制新增任务不能当作扫完；未选中的图文也应计入已发现总量: catalog=%v complete=%v tasks=%d", catalog, complete, len(s.Tasks))
	}
	if err := a.transaction(func() error {
		a.state.Creators[0].ContentCatalog["video:removed"] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.StartHistory(c.ID, HistoryInput{WatchVideos: true, WatchDynamics: true}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		readyHistory(t, a, c.ID)
		if err := a.ProcessHistoryPage(context.Background(), c.ID); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if err := a.recordContentCatalog(c.ID, bilibiliFeed{Videos: []Entry{{ID: "BVnewDuringHistory"}}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	s = a.Snapshot()
	catalog, complete = contentCatalog(t, s.Creators[0])
	if len(catalog) != 5 || !catalog["video:BVnewDuringHistory"] || !complete || len(s.Tasks) != 4 {
		t.Fatalf("完整历史应按内容 ID 去重并分别覆盖两种内容: catalog=%v complete=%v tasks=%d", catalog, complete, len(s.Tasks))
	}
	b, err := New(filepath.Dir(a.statePath), a.downloadDir, a.binary)
	if err != nil {
		t.Fatal(err)
	}
	catalog, complete = contentCatalog(t, b.Snapshot().Creators[0])
	if len(catalog) != 5 || !complete {
		t.Fatal("总量及完整性必须随状态持久化")
	}
}

func TestContentCatalogNormalScanDoesNotCountBaselineAsPulled(t *testing.T) {
	a := testApp(t)
	useTestCookies(t, a)
	c, _ := a.AddCreator(CreatorInput{URL: "123", Interval: 30, Quality: "best", WatchDynamics: true})
	a.httpClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"code":0,"data":{"has_more":false,"items":[{"id_str":"v","type":"DYNAMIC_TYPE_AV","modules":{"module_dynamic":{"major":{"archive":{"bvid":"BVbaseline"}}}}},{"id_str":"d","type":"DYNAMIC_TYPE_WORD","modules":{"module_dynamic":{"desc":{"text":"基线"}}}}]}}`
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	if err := a.ScanCreator(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	s := a.Snapshot()
	catalog, complete := contentCatalog(t, s.Creators[0])
	if len(catalog) != 2 || !complete || len(s.Tasks) != 0 {
		t.Fatalf("基线内容应计入全部，但不能算已拉取: catalog=%v complete=%v tasks=%d", catalog, complete, len(s.Tasks))
	}
}
