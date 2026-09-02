package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

// Event is the document we index. A synthetic log event keeps the demo
// self-contained: no dataset to download, and ~200 bytes per document is
// typical for logs and metrics payloads.
type Event struct {
	ID        string    `json:"-"`
	Timestamp time.Time `json:"@timestamp"`
	Service   string    `json:"service"`
	Level     string    `json:"level"`
	Region    string    `json:"region"`
	LatencyMS int       `json:"latency_ms"`
	Message   string    `json:"message"`
}

var (
	services = []string{"booking", "tracking", "billing", "customs", "vessel"}
	levels   = []string{"INFO", "INFO", "INFO", "WARN", "ERROR"}
	regions  = []string{"eu-west-1", "us-west-2", "ap-south-1"}
)

// generate produces n events into out and closes it. Sending blocks whenever
// the channel is full: this is the first point of backpressure in the pipeline.
func generate(ctx context.Context, n int, out chan<- Event) {
	defer close(out)
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < n; i++ {
		ev := Event{
			ID:        fmt.Sprintf("evt-%09d", i),
			Timestamp: time.Now(),
			Service:   services[r.IntN(len(services))],
			Level:     levels[r.IntN(len(levels))],
			Region:    regions[r.IntN(len(regions))],
			LatencyMS: r.IntN(900) + 5,
			Message:   fmt.Sprintf("request %d processed", i),
		}
		select {
		case out <- ev:
		case <-ctx.Done():
			return
		}
	}
}
