package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
)

// encodeBulk turns a batch into the NDJSON body the _bulk API expects: one
// action line followed by one document line per event. Sending our own
// document IDs makes a retry overwrite the document instead of duplicating it.
// #region encode
func encodeBulk(batch []Event) (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	for _, ev := range batch {
		fmt.Fprintf(buf, `{"index":{"_id":%q}}`+"\n", ev.ID)
		if err := enc.Encode(ev); err != nil { // Encode appends the newline
			return nil, err
		}
	}
	return buf, nil
}

// #endregion encode

// failure is one document that OpenSearch did not accept.
type failure struct {
	Event  Event
	Status int
	Reason string
}

// sendBulk sends one batch and reports which documents were rejected.
// HTTP 200 does not mean success: with errors=true every item carries its
// own status, and 429 means the write thread pool queue was full.
func sendBulk(ctx context.Context, client *opensearchapi.Client, index string, batch []Event, stats *Stats) ([]failure, error) {
	body, err := encodeBulk(batch)
	if err != nil {
		return nil, err
	}
	stats.Requests.Add(1)
	stats.Bytes.Add(int64(body.Len()))

	// #region send
	resp, err := client.Bulk(ctx, opensearchapi.BulkReq{Index: index, Body: body})
	if err != nil { // the whole request failed: the caller retries the batch
		log.Printf("bulk request failed: %v", err)
		return nil, err
	}
	if !resp.Errors {
		return nil, nil
	}
	var failed []failure
	for i, item := range resp.Items { // items come back in submission order
		r := item["index"]
		if r.Error == nil && r.Status < 300 {
			continue
		}
		f := failure{Event: batch[i], Status: r.Status}
		if r.Error != nil {
			f.Reason = r.Error.Type + ": " + r.Error.Reason
		}
		failed = append(failed, f)
	}
	return failed, nil
}

// #endregion send
