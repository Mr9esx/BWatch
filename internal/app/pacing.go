package app

import (
	"net/url"
	"strings"
	"time"
)

func isBilibiliTask(t Task) bool {
	if t.Platform != "" {
		return t.Platform == PlatformBilibili
	}
	u, err := url.Parse(t.URL)
	return err == nil && (u.Hostname() == "bilibili.com" || strings.HasSuffix(u.Hostname(), ".bilibili.com"))
}
func (a *App) bilibiliBlocked(now time.Time) bool {
	return a.platformBlocked(PlatformBilibili, now)
}

func (a *App) platformBlocked(platform PlatformID, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return future(a.platformBlockedUntilLocked(platform), now)
}

func platformCooldown(state State, platform PlatformID) *time.Time {
	if until, ok := state.PlatformCooldowns[platform]; ok {
		return &until
	}
	if platform == PlatformBilibili {
		return state.BilibiliBlockedUntil
	}
	return nil
}

func (a *App) platformBlockedUntilLocked(platform PlatformID) *time.Time {
	return platformCooldown(a.state, platform)
}

func isPlatformRiskError(platform PlatformID, message string) bool {
	if platform == PlatformBilibili {
		return isRiskError(message)
	}
	if platform != PlatformXiaohongshu {
		return false
	}
	message = strings.ToLower(message)
	for _, token := range []string{"http 403", "http 406", "http 412", "http 429", "额外验证", "验证码", "要求验证", "限制了当前网络", "平台限制", "captcha", "verify", "频繁", "风控", "blocked", "forbidden"} {
		if strings.Contains(message, token) {
			return true
		}
	}
	return false
}

// Call within a transaction. Cooldowns are isolated by platform and survive restarts.
func (a *App) blockBilibiliLocked(message string, now time.Time) {
	a.blockPlatformLocked(PlatformBilibili, message, now)
}

func (a *App) blockPlatformLocked(platform PlatformID, message string, now time.Time) {
	if !isPlatformRiskError(platform, message) {
		return
	}
	if a.state.PlatformCooldowns == nil {
		a.state.PlatformCooldowns = map[PlatformID]time.Time{}
	}
	if a.state.PlatformRiskCounts == nil {
		a.state.PlatformRiskCounts = map[PlatformID]int{}
	}
	previous := a.platformBlockedUntilLocked(platform)
	if previous == nil || now.Sub(*previous) > 24*time.Hour {
		a.state.PlatformRiskCounts[platform] = 0
	}
	a.state.PlatformRiskCounts[platform]++
	delay := 30 * time.Minute
	for i := 1; i < a.state.PlatformRiskCounts[platform] && delay < 6*time.Hour; i++ {
		delay *= 2
	}
	if delay > 6*time.Hour {
		delay = 6 * time.Hour
	}
	until := now.Add(delay)
	if previous != nil && previous.After(until) {
		until = *previous
	}
	a.state.PlatformCooldowns[platform] = until
	if platform == PlatformBilibili {
		a.state.BilibiliBlockedUntil = &until
		a.state.BilibiliRiskCount = a.state.PlatformRiskCounts[platform]
	}
	a.event("error", contentPlatformLabel(platform)+"触发平台限制，暂停该平台新下载与列表请求至 "+until.Format("01-02 15:04"))
}
