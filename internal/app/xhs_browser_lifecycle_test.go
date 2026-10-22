package app

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

func TestXHSBrowserSurvivesRepeatedOperations(t *testing.T) {
	if os.Getenv("BILIFETCH_BROWSER_TEST") != "1" {
		t.Skip("set BILIFETCH_BROWSER_TEST=1 to run the local headless browser regression")
	}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	options := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	options = append(options, chromedp.UserDataDir(t.TempDir()))
	allocator, allocCancel := chromedp.NewExecAllocator(context.Background(), options...)
	browser, browserCancel := chromedp.NewContext(allocator)
	b := &xhsBrowser{browser: browser, browserStop: browserCancel, allocCancel: allocCancel}
	defer b.close()
	if err := initializeXHSPage(ctx, browser, browserCancel); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := b.start(ctx); err != nil || b.browser != browser {
			t.Fatalf("healthy browser was restarted: %v", err)
		}
	}
	b.mu.Lock()
	page, err := b.pageLocked(ctx)
	b.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		operation, cancel := context.WithTimeout(page, 3*time.Second)
		var result int
		err := chromedp.Run(operation, chromedp.Evaluate("2 + 2", &result))
		cancel()
		if err != nil || result != 4 {
			t.Fatalf("operation %d lost its target event loop: %v", i, err)
		}
	}
	err = storage.SetCookies([]*network.CookieParam{
		{Name: "a1", Value: "fixture", Domain: ".xiaohongshu.com", Path: "/"},
		{Name: "web_session", Value: "fixture", Domain: ".xiaohongshu.com", Path: "/"},
	}).Do(cdp.WithExecutor(ctx, chromedp.FromContext(browser).Browser))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		cookies, err := b.cookieSnapshot(ctx)
		if err != nil || xhsAccountFromCookies(cookies).State != "logged_in" {
			t.Fatalf("cookie read %d failed: %v", i, err)
		}
	}
}
