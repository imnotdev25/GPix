package bridge

import (
	"fmt"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

type editThrottle struct {
	msg    *telegram.NewMessage
	every  time.Duration
	mu     sync.Mutex
	lastAt time.Time
	lastTx string
}

func newEditThrottle(msg *telegram.NewMessage, every time.Duration) *editThrottle {
	return &editThrottle{msg: msg, every: every}
}

func (t *editThrottle) Force(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if text == t.lastTx {
		return
	}
	t.lastTx = text
	t.lastAt = time.Now()
	if t.msg != nil {
		_, _ = t.msg.Edit(text)
	}
}

func (t *editThrottle) Tick(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if text == t.lastTx {
		return
	}
	if time.Since(t.lastAt) < t.every {
		return
	}
	t.lastTx = text
	t.lastAt = time.Now()
	if t.msg != nil {
		_, _ = t.msg.Edit(text)
	}
}

func fmtBytes(n int64) string {
	const (
		KB = 1 << 10
		MB = 1 << 20
		GB = 1 << 30
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.2f GB", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(MB))
	case n >= KB:
		return fmt.Sprintf("%.0f KB", float64(n)/float64(KB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func fmtPct(done, total int64) string {
	if total <= 0 {
		return fmtBytes(done)
	}
	pct := float64(done) * 100 / float64(total)
	return fmt.Sprintf("%.1f%% (%s / %s)", pct, fmtBytes(done), fmtBytes(total))
}
