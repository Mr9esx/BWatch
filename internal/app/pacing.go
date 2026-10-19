package app

import (
	"net/url"
	"strings"
	"time"
)

func isBilibiliTask(t Task) bool {
	u, err := url.Parse(t.URL)
	return err == nil && (u.Hostname() == "bilibili.com" || strings.HasSuffix(u.Hostname(), ".bilibili.com"))
}
func (a *App) bilibiliBlocked(now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return future(a.state.BilibiliBlockedUntil, now)
}

// Call within a transaction. Cooldown applies to every Bilibili creator and survives restarts.
func (a *App) blockBilibiliLocked(message string, now time.Time) {
	if !isRiskError(message) {
		return
	}
	if a.state.BilibiliBlockedUntil == nil || now.Sub(*a.state.BilibiliBlockedUntil) > 24*time.Hour {
		a.state.BilibiliRiskCount = 0
	}
	a.state.BilibiliRiskCount++
	delay := 30 * time.Minute
	for i := 1; i < a.state.BilibiliRiskCount && delay < 6*time.Hour; i++ {
		delay *= 2
	}
	if delay > 6*time.Hour {
		delay = 6 * time.Hour
	}
	until := now.Add(delay)
	if a.state.BilibiliBlockedUntil == nil || until.After(*a.state.BilibiliBlockedUntil) {
		a.state.BilibiliBlockedUntil = &until
	}
	a.event("error", "B 站触发风控，暂停新下载与列表请求至 "+a.state.BilibiliBlockedUntil.Format("01-02 15:04"))
}
