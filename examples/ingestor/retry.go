package main

import (
	"math/rand/v2"
	"time"
)

const (
	baseDelay = 200 * time.Millisecond // first retry waits about this long
	maxDelay  = 10 * time.Second       // never sleep longer than this per attempt
)

// backoff returns how long to wait before retry number attempt (1-based).
// Exponential growth with full jitter, capped at maxDelay. Jitter spreads
// retries out so many workers do not hit the cluster at the same instant.
// #region backoff
func backoff(attempt int) time.Duration {
	d := baseDelay << (attempt - 1)
	if d <= 0 || d > maxDelay {
		d = maxDelay
	}
	return baseDelay/2 + time.Duration(rand.Int64N(int64(d)))
}

// #endregion backoff

// retryable reports whether a per-item bulk status is worth retrying.
// 429 is rejected_execution_exception: the write thread pool queue is full.
// (Elasticsearch calls it es_rejected_execution_exception; OpenSearch dropped
// the prefix when it forked, so that is the name you see on an OpenSearch cluster.)
// 503 is a node that is temporarily unavailable. Everything else (400 mapping
// errors, 404 missing index) will fail again, so it goes to the dead-letter file.
// #region retryable
func retryable(status int) bool {
	return status == 429 || status == 503
}

// #endregion retryable
