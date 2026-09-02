package main

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

// Stats are updated atomically by every worker and printed once a second.
type Stats struct {
	Indexed  atomic.Int64 // documents acknowledged by OpenSearch
	Retried  atomic.Int64 // documents re-sent after a retryable failure
	Failed   atomic.Int64 // documents written to the dead-letter file
	Requests atomic.Int64 // bulk requests sent
	Bytes    atomic.Int64 // bulk request bytes sent
}

// report prints a throughput line every interval until ctx is done.
func (s *Stats) report(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	var last int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cur := s.Indexed.Load()
			log.Printf("indexed=%d rate=%d docs/s requests=%d retried=%d failed=%d",
				cur, (cur-last)/int64(interval/time.Second),
				s.Requests.Load(), s.Retried.Load(), s.Failed.Load())
			last = cur
		}
	}
}
