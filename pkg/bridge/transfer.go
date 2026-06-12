package bridge

import (
	"context"
	"errors"
	"os"
)

type Transfer struct {
	TempDir string
	Sem     chan struct{}
}

func NewTransfer(tempDir string, maxConcurrent int) *Transfer {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &Transfer{TempDir: tempDir, Sem: make(chan struct{}, maxConcurrent)}
}

func (t *Transfer) Acquire(ctx context.Context) (func(), error) {
	select {
	case t.Sem <- struct{}{}:
		return func() { <-t.Sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *Transfer) Temp(prefix string) (*os.File, func(), error) {
	if t.TempDir == "" {
		return nil, nil, errors.New("bridge: TempDir is empty")
	}
	f, err := os.CreateTemp(t.TempDir, prefix+"*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}
	return f, cleanup, nil
}
