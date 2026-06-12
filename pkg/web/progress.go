package web

import (
	"sync"
)

type progressBus struct {
	mu   sync.Mutex
	subs map[string]chan string
}

func newProgressBus() *progressBus {
	return &progressBus{subs: make(map[string]chan string)}
}

func (b *progressBus) Subscribe(id string) chan string {
	ch := make(chan string, 32)
	b.mu.Lock()
	b.subs[id] = ch
	b.mu.Unlock()
	return ch
}

func (b *progressBus) Unsubscribe(id string) {
	b.mu.Lock()
	if ch, ok := b.subs[id]; ok {
		delete(b.subs, id)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *progressBus) Publish(id, msg string) {
	b.mu.Lock()
	ch, ok := b.subs[id]
	b.mu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- msg:
	default:
	}
}
