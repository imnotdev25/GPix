package gpmc

import (
	"io"
	"sync/atomic"
	"time"
)

type progressReader struct {
	rc      io.ReadCloser
	total   int64
	read    int64
	emit    func(done int64)
	lastAt  time.Time
	lastVal int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.rc.Read(b)
	if n > 0 {
		done := atomic.AddInt64(&p.read, int64(n))
		now := time.Now()
		if p.emit != nil && (done == p.total || now.Sub(p.lastAt) >= time.Second) {
			p.lastAt = now
			p.lastVal = done
			p.emit(done)
		}
	}
	return n, err
}

func (p *progressReader) Close() error { return p.rc.Close() }

func NewProgressReader(rc io.ReadCloser, total int64, emit func(done int64)) io.ReadCloser {
	return &progressReader{rc: rc, total: total, emit: emit}
}
