// Command naive is the loader most projects start with: one document, one
// HTTP request, one at a time. It exists so the talk has a baseline.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"time"

	"github.com/opensearch-project/opensearch-go/v4"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

type Event struct {
	Timestamp time.Time `json:"@timestamp"`
	Service   string    `json:"service"`
	Level     string    `json:"level"`
	Region    string    `json:"region"`
	LatencyMS int       `json:"latency_ms"`
	Message   string    `json:"message"`
}

func main() {
	addr := flag.String("addr", "http://localhost:9200", "OpenSearch URL")
	user := flag.String("user", "", "basic auth username")
	pass := flag.String("pass", "", "basic auth password")
	index := flag.String("index", "events-naive", "index to load")
	docs := flag.Int("docs", 20_000, "number of documents to index")
	flag.Parse()

	client, err := opensearchapi.NewClient(opensearchapi.Config{Client: opensearch.Config{
		Addresses: []string{*addr}, Username: *user, Password: *pass, InsecureSkipVerify: true,
	}})
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	start := time.Now()
	for i := range *docs {
		ev := Event{Timestamp: time.Now(), Service: "booking", Level: "INFO", Region: "us-west-2",
			LatencyMS: rand.IntN(900) + 5, Message: fmt.Sprintf("request %d processed", i)}
		body, _ := json.Marshal(ev)
		_, err := client.Index(ctx, opensearchapi.IndexReq{
			Index: *index, DocumentID: fmt.Sprintf("evt-%09d", i), Body: bytes.NewReader(body),
		})
		if err != nil {
			log.Fatalf("index doc %d: %v", i, err)
		}
		if (i+1)%5000 == 0 {
			log.Printf("indexed=%d rate=%.0f docs/s", i+1, float64(i+1)/time.Since(start).Seconds())
		}
	}
	elapsed := time.Since(start)
	log.Printf("done: indexed=%d in %s (%.0f docs/s)", *docs, elapsed.Round(time.Millisecond), float64(*docs)/elapsed.Seconds())
}
