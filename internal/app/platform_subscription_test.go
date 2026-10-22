package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreatorNumberUsesSelectedPlatform(t *testing.T) {
	for _, tc := range []struct {
		platform PlatformID
		wantURL  string
	}{
		{PlatformXiaohongshu, "xhsid:875230756"},
		{PlatformBilibili, "https://space.bilibili.com/875230756/video"},
	} {
		t.Run(string(tc.platform), func(t *testing.T) {
			ref, err := newPlatformRegistry().normalizeCreatorForPlatform(tc.platform, "875230756")
			if err != nil || ref.Platform != tc.platform || ref.URL != tc.wantURL {
				t.Fatalf("selected platform lost: %+v, %v", ref, err)
			}
		})
	}
	if _, err := newPlatformRegistry().normalizeCreatorForPlatform(PlatformXiaohongshu, "https://space.bilibili.com/875230756/video"); err == nil {
		t.Fatal("a conflicting link must not silently switch platforms")
	}
}

func TestSubscriptionHTTPHonorsPlatformAndSavedCredential(t *testing.T) {
	credentialKeyCache.Lock()
	previousKey := credentialKeyCache.key
	credentialKeyCache.key = bytes.Repeat([]byte{7}, 32)
	credentialKeyCache.Unlock()
	defer func() {
		credentialKeyCache.Lock()
		credentialKeyCache.key = previousKey
		credentialKeyCache.Unlock()
	}()
	a, err := New(t.TempDir(), t.TempDir(), "yt-dlp")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.saveCredential(PlatformXiaohongshu, []byte("fixture-session")); err != nil {
		t.Fatal(err)
	}
	handler := a.Handler(context.Background(), "", "")
	input := CreatorInput{Platform: PlatformXiaohongshu, URL: "https://www.xiaohongshu.com/user/profile/5da907190000000001005c5d", Interval: 10, Quality: "best", WatchDynamics: true}
	body, _ := json.Marshal(input)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("POST", "/api/creators", bytes.NewReader(body)))
	var creator Creator
	if err := json.Unmarshal(response.Body.Bytes(), &creator); err != nil {
		t.Fatal(err)
	}
	if response.Code != 201 || creator.Platform != PlatformXiaohongshu {
		t.Fatalf("XHS subscription rejected or misrouted: %d %s", response.Code, response.Body.String())
	}
	if a.xhsBrowser.browser != nil {
		t.Fatal("adding a profile URL must not launch a browser for login verification")
	}
	// Exercise the full numeric-ID HTTP path with a deterministic API worker.
	// Only the XHS credential exists: routing this number to Bilibili must fail.
	runtimeDir := t.TempDir()
	binDir := filepath.Join(runtimeDir, "venv", "bin")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(t.TempDir(), "request.json")
	t.Setenv("BILIFETCH_XHS_RUNTIME", runtimeDir)
	t.Setenv("BILIFETCH_TEST_CAPTURE", capture)
	worker := "#!/bin/sh\ncat > \"$BILIFETCH_TEST_CAPTURE\"\nprintf '%s' '{\"ok\":true,\"data\":{\"id\":\"5da907190000000001005c5e\",\"redID\":\"875230756\",\"name\":\"咬太阳Bite\"}}'\n"
	if err := os.WriteFile(filepath.Join(binDir, "python"), []byte(worker), 0700); err != nil {
		t.Fatal(err)
	}
	input.URL = "875230756"
	body, _ = json.Marshal(input)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("POST", "/api/creators", bytes.NewReader(body)))
	if response.Code != 201 {
		t.Fatalf("numeric XHS ID failed: %d %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &creator); err != nil || creator.Platform != PlatformXiaohongshu || creator.Name != "咬太阳Bite" {
		t.Fatalf("numeric XHS response lost platform or name: %+v, %v", creator, err)
	}
	requestData, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	var apiRequest xhsAPIRequest
	if err := json.Unmarshal(requestData, &apiRequest); err != nil || apiRequest.Operation != "resolve_creator" || apiRequest.RedID != "875230756" {
		t.Fatalf("numeric XHS ID was not sent to its API worker: %v", err)
	}
	if a.xhsBrowser.browser != nil {
		t.Fatal("resolving a Xiaohongshu number launched a browser")
	}
	input.URL = creator.URL + "?xsec_token=changed"
	body, _ = json.Marshal(input)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("POST", "/api/creators", bytes.NewReader(body)))
	if response.Code != 400 || !strings.Contains(response.Body.String(), "已经订阅") {
		t.Fatal("a refreshed security token created a duplicate subscription")
	}
	input.Platform, input.URL = PlatformBilibili, "875230756"
	body, _ = json.Marshal(input)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("POST", "/api/creators", bytes.NewReader(body)))
	if response.Code != 400 || !strings.Contains(response.Body.String(), "B 站") {
		t.Fatalf("XHS credential must not authorize Bilibili: %d %s", response.Code, response.Body.String())
	}
}
