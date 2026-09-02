package main

import (
	"context"
	"sync"
	"time"
)

// batcher groups events into slices of size, or whatever has arrived when
// flush elapses, so a slow producer never leaves a partial batch waiting.
// #region batcher
func batcher(ctx context.Context, in <-chan Event, out chan<- []Event, size int, flush time.Duration) {
	defer close(out)
	buf := make([]Event, 0, size)
	t := time.NewTicker(flush)
	defer t.Stop()

	emit := func() {
		if len(buf) == 0 {
			return
		}
		select {
		case out <- buf: // blocks while every worker is busy: backpressure
		case <-ctx.Done():
		}
		buf = make([]Event, 0, size)
	}
	for {
		select {
		case ev, ok := <-in:
			if !ok {
				emit()
				return
			}
			buf = append(buf, ev)
			if len(buf) == size {
				emit()
			}
		case <-t.C:
			emit()
		case <-ctx.Done():
			return
		}
	}
}

// #endregion batcher

// runWorkers starts n goroutines that each take batches until the channel
// is closed. The channel capacity is the only queue in the system: when the
// workers fall behind it fills, the batcher blocks, and the generator blocks.
// #region workers
func runWorkers(n int, batches <-chan []Event, work func([]Event)) {
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			for b := range batches {
				work(b)
			}
		})
	}
	wg.Wait()
}

// #endregion workers
