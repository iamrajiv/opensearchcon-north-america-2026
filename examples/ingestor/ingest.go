package main

import (
	"context"
	"time"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

// Ingestor holds what every worker shares: the client and the counters.
type Ingestor struct {
	client     *opensearchapi.Client
	index      string
	retryFor   time.Duration // keep retrying rejected documents this long
	maxTries   int           // or, if non-zero, give up after this many attempts
	stats      *Stats
	deadLetter *DeadLetter
}

// keepTrying decides whether a rejected document gets another attempt. The
// default is a time window: a 429 means the cluster is busy, and busy passes.
// Setting -max-attempts swaps in the fixed-attempt rule most retry loops use,
// which is how the talk shows a cluster that is merely slow being mistaken for
// a cluster that is broken.
func (in *Ingestor) keepTrying(attempt int, started time.Time) bool {
	if in.maxTries > 0 {
		return attempt < in.maxTries
	}
	return time.Since(started) < in.retryFor
}

// indexBatch sends a batch and keeps re-sending only the rejected documents,
// backing off exponentially, until they are accepted or the retry window
// closes. Nothing is dropped: what cannot be indexed goes to the dead-letter file.
// #region retry
func (in *Ingestor) indexBatch(ctx context.Context, batch []Event) {
	started := time.Now()
	for attempt := 1; len(batch) > 0; attempt++ {
		failed, err := sendBulk(ctx, in.client, in.index, batch, in.stats)
		if err != nil { // the whole request failed: every document is retryable
			failed = asFailures(batch, 503, err.Error())
		}
		in.stats.Indexed.Add(int64(len(batch) - len(failed)))
		batch = batch[:0]
		for _, f := range failed {
			if retryable(f.Status) && in.keepTrying(attempt, started) {
				batch = append(batch, f.Event)
				continue
			}
			in.deadLetter.Write(f)
			in.stats.Failed.Add(1)
		}
		if len(batch) > 0 {
			in.stats.Retried.Add(int64(len(batch)))
			sleep(ctx, backoff(attempt))
		}
	}
}

// #endregion retry

// asFailures marks a whole batch as failed with one status, used when the
// request itself failed and no per-item response exists.
func asFailures(batch []Event, status int, reason string) []failure {
	out := make([]failure, len(batch))
	for i, ev := range batch {
		out[i] = failure{Event: ev, Status: status, Reason: reason}
	}
	return out
}

// sleep waits for d unless the context is cancelled first.
func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}
