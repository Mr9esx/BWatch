package app

import (
	"context"
	"io"
	"time"
)

// MB/s 使用十进制字节，视频和图文共用相同换算。
func downloadBytesPerSecond(s Settings) int64 { return int64(s.MaxDownloadSpeed * 1_000_000) }

type pacedReader struct {
	ctx    context.Context
	source io.Reader
	rate   int64
	next   time.Time
}

func (r *pacedReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	// 小块读取限制突发，并让取消能够及时打断限速等待。
	chunk := max(1, min(int64(32<<10), r.rate/10))
	if int64(len(p)) > chunk {
		p = p[:chunk]
	}
	if now := time.Now(); r.next.Before(now) {
		r.next = now
	}
	n, err := r.source.Read(p)
	r.next = r.next.Add(time.Duration(n) * time.Second / time.Duration(r.rate))
	if delay := time.Until(r.next); delay > 0 && n > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		case <-timer.C:
		}
	}
	return n, err
}
