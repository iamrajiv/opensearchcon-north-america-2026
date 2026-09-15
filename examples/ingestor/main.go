// Command ingestor loads synthetic events into OpenSearch as fast as the
// cluster will accept them: a worker pool of bulk requests, bounded channels
// for backpressure, and per-document retries with a dead-letter file.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"
)

func main() {
	addr := flag.String("addr", "http://localhost:9200", "OpenSearch URL")
	user := flag.String("user", "", "basic auth username (empty when security is disabled)")
	pass := flag.String("pass", "", "basic auth password")
	index := flag.String("index", "events", "index to (re)create and load")
	docs := flag.Int("docs", 1_000_000, "number of documents to generate")
	workers := flag.Int("workers", 8, "parallel bulk requests in flight")
	batchSize := flag.Int("batch", 2000, "documents per bulk request")
	queue := flag.Int("queue", 4, "batches buffered ahead of the workers")
	shards := flag.Int("shards", 1, "primary shards for the index")
	retryFor := flag.Duration("retry-for", 2*time.Minute, "keep retrying rejected documents this long before dead-lettering")
	maxTries := flag.Int("max-attempts", 0, "give up after this many attempts instead of using the -retry-for window")
	dlq := flag.String("dead-letter", "dead-letter.ndjson", "where rejected documents are written")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	client, err := newClient(*addr, *user, *pass, *workers)
	if err != nil {
		log.Fatalf("client: %v", err)
	}
	if err := prepareIndex(ctx, client, *index, *shards); err != nil {
		log.Fatalf("prepare index: %v", err)
	}
	dl, err := newDeadLetter(*dlq)
	if err != nil {
		log.Fatalf("dead letter: %v", err)
	}
	defer dl.Close()

	in := &Ingestor{client: client, index: *index, retryFor: *retryFor, maxTries: *maxTries, stats: &Stats{}, deadLetter: dl}

	// #region pipeline
	// The pipeline: generate -> events -> batcher -> batches -> workers -> _bulk
	events := make(chan Event, *batchSize)
	batches := make(chan []Event, *queue)
	go generate(ctx, *docs, events)
	go batcher(ctx, events, batches, *batchSize, 500*time.Millisecond)
	// #endregion pipeline

	reportCtx, stopReport := context.WithCancel(ctx)
	go in.stats.report(reportCtx, time.Second)

	start := time.Now()
	runWorkers(*workers, batches, func(b []Event) { in.indexBatch(ctx, b) })
	elapsed := time.Since(start)
	stopReport()

	count, err := finishIndex(ctx, client, *index)
	if err != nil {
		log.Fatalf("finish index: %v", err)
	}
	s := in.stats
	log.Printf("done: indexed=%d failed=%d requests=%d retried=%d in %s",
		s.Indexed.Load(), s.Failed.Load(), s.Requests.Load(), s.Retried.Load(), elapsed.Round(time.Millisecond))
	log.Printf("throughput: %.0f docs/s, %.1f MB/s; %s now holds %d documents",
		float64(s.Indexed.Load())/elapsed.Seconds(), float64(s.Bytes.Load())/1e6/elapsed.Seconds(), *index, count)
}
