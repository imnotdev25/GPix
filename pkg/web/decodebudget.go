package web

import (
	"bytes"
	"context"
	"image"
	"io"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// Decoding a full-size photo holds every pixel in memory: a 36 MP HEIC takes
// about 1 GB while it decodes. Several thumbnail renders at once (background
// workers plus the viewer's large previews) used to add up to several GB, so
// every decode now takes a share of one process-wide pixel budget.
const (
	decodeUnitPixels  = 1 << 22 // budget unit: about 4 megapixels
	decodeBudgetUnits = 10      // about 40 MP decoding at once; a bigger photo decodes alone
	decodeIdleRelease = 10 * time.Second
)

var decodeBudget = newPixelBudget(decodeBudgetUnits)

type pixelBudget struct {
	tokens   chan struct{}
	mu       sync.Mutex // one waiter takes tokens at a time, so two big decodes can't each hold half
	inflight atomic.Int64
	idleGen  atomic.Uint64
	freeing  atomic.Bool
}

func newPixelBudget(units int) *pixelBudget {
	b := &pixelBudget{tokens: make(chan struct{}, units)}
	b.put(units)
	return b
}

func (b *pixelBudget) put(n int) {
	for i := 0; i < n; i++ {
		b.tokens <- struct{}{}
	}
}

// acquire waits until an image of pixels fits in the budget, and returns the
// func that gives the share back.
func (b *pixelBudget) acquire(ctx context.Context, pixels int) (func(), error) {
	n := min(max((pixels+decodeUnitPixels-1)/decodeUnitPixels, 1), cap(b.tokens))
	b.mu.Lock()
	for got := 0; got < n; got++ {
		select {
		case <-b.tokens:
		case <-ctx.Done():
			b.mu.Unlock()
			b.put(got)
			return nil, ctx.Err()
		}
	}
	b.mu.Unlock()
	b.inflight.Add(1)
	var once sync.Once
	return func() {
		once.Do(func() {
			b.put(n)
			// The HEIC decoder keeps each decoder instance, grown to the size
			// of the photo (about 1 GB for 36 MP), in a sync.Pool, one per CPU.
			// Clear it after every large decode instead of letting several
			// such instances pile up.
			if n >= 2 {
				go b.freeNow()
			}
			b.finished()
		})
	}, nil
}

// freeNow clears sync.Pools (they survive one collection, so it takes two)
// and returns the freed memory to the OS. Decode buffers hold no pointers,
// so these collections are quick.
func (b *pixelBudget) freeNow() {
	if !b.freeing.CompareAndSwap(false, true) {
		return
	}
	defer b.freeing.Store(false)
	runtime.GC()
	debug.FreeOSMemory()
}

// finished hands freed memory back to the OS once no decode has run for a
// few seconds. Go returns freed heap slowly, so after a burst of renders the
// process would otherwise keep looking large in top or docker stats.
func (b *pixelBudget) finished() {
	if b.inflight.Add(-1) != 0 {
		return
	}
	gen := b.idleGen.Add(1)
	time.AfterFunc(decodeIdleRelease, func() {
		if b.inflight.Load() == 0 && b.idleGen.Load() == gen {
			b.freeNow()
		}
	})
}

// decodeLimited decodes an image (at most thumbCacheMaxBytes of input) inside
// the pixel budget. Call release as soon as the full-size image isn't needed.
func decodeLimited(ctx context.Context, r io.Reader) (img image.Image, release func(), err error) {
	data, err := io.ReadAll(io.LimitReader(r, thumbCacheMaxBytes))
	if err != nil {
		return nil, nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	release, err = decodeBudget.acquire(ctx, cfg.Width*cfg.Height)
	if err != nil {
		return nil, nil, err
	}
	img, _, err = image.Decode(bytes.NewReader(data))
	if err != nil {
		release()
		return nil, nil, err
	}
	return img, release, nil
}
