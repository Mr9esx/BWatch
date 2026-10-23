package app

import (
	"context"
	_ "embed"
	"errors"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const xhsBucketCapacity = 1.0

// Full scans read up to 100 pages. Include bootstrap calls and time for
// pagination/network overhead, using the same budget for both Go contexts.
func xhsHistoryScanTimeout(qps float64) time.Duration {
	if math.IsNaN(qps) || math.IsInf(qps, 0) || qps < 0.05 {
		qps = 0.05
	}
	return max(20*time.Minute, time.Duration(105/qps*float64(time.Second))+10*time.Minute)
}

func isXHSAPIURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host != "xiaohongshu.com" && !strings.HasSuffix(host, ".xiaohongshu.com") {
		return false
	}
	return strings.HasPrefix(host, "api.") || strings.HasPrefix(host, "edith.") || strings.HasPrefix(host, "as.") || strings.HasPrefix(host, "passport.") || strings.HasPrefix(strings.ToLower(u.Path), "/api/") || strings.HasPrefix(strings.ToLower(u.Path), "/web_api/") || strings.HasPrefix(strings.ToLower(u.Path), "/app-api/")
}

func isRiskHTTPStatus(status int64) bool {
	return status == 403 || status == 406 || status == 412 || status == 429
}

//go:embed xhs_rate_limiter.py
var xhsRateLimiterSource []byte

//go:embed xhs_sitecustomize.py
var xhsSitecustomizeSource []byte

func (a *App) writeXHSPythonHooks() error {
	if err := os.MkdirAll(a.xhsPythonDir, 0700); err != nil {
		return err
	}
	for name, data := range map[string][]byte{"xhs_rate_limiter.py": xhsRateLimiterSource, "sitecustomize.py": xhsSitecustomizeSource} {
		path := filepath.Join(a.xhsPythonDir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) configureXHSPython(cmd *exec.Cmd, ytDLP bool) {
	env := os.Environ()
	set := func(key, value string) {
		prefix := key + "="
		filtered := env[:0]
		for _, item := range env {
			if !strings.HasPrefix(item, prefix) {
				filtered = append(filtered, item)
			}
		}
		env = append(filtered, prefix+value)
	}
	pythonPath := a.xhsPythonDir
	if inherited := os.Getenv("PYTHONPATH"); inherited != "" {
		pythonPath += string(os.PathListSeparator) + inherited
	}
	set("PYTHONPATH", pythonPath)
	set("BILIFETCH_XHS_RATE_DB", a.xhsRateDBPath)
	if ytDLP {
		set("BILIFETCH_XHS_YTDLP_LIMIT", "1")
	} else {
		set("BILIFETCH_XHS_YTDLP_LIMIT", "")
	}
	cmd.Env = env
}

// waitXHSAPIToken shares one persistent bucket with Python workers using the
// same SQLite database. The write lock is held only while updating the bucket.
func (a *App) waitXHSAPIToken(ctx context.Context) error {
	for {
		conn, err := a.stateDB.Conn(ctx)
		if err != nil {
			return err
		}
		if _, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
			conn.Close()
			return err
		}
		var tokens, updatedAt, qps float64
		err = conn.QueryRowContext(ctx, `SELECT tokens, updated_at, qps FROM xhs_api_bucket WHERE id = 1`).Scan(&tokens, &updatedAt, &qps)
		if err == nil && (qps <= 0 || math.IsNaN(qps) || math.IsInf(qps, 0)) {
			err = errors.New("小红书 API QPS 配置无效")
		}
		now := float64(time.Now().UnixNano()) / 1e9
		wait := time.Duration(0)
		if err == nil {
			elapsed := max(0.0, now-updatedAt)
			tokens = min(xhsBucketCapacity, tokens+elapsed*qps)
			if tokens >= 1 {
				tokens--
			} else {
				wait = time.Duration((1 - tokens) / qps * float64(time.Second))
			}
			_, err = conn.ExecContext(ctx, `UPDATE xhs_api_bucket SET tokens = ?, updated_at = ? WHERE id = 1`, tokens, now)
		}
		if err == nil {
			_, err = conn.ExecContext(ctx, `COMMIT`)
		} else {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
		_ = conn.Close()
		if err != nil {
			return err
		}
		if wait <= 0 {
			return nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
