# High throughput OpenSearch ingestor

The code shown in the talk. The `// #region` comments mark the exact lines the
slides import, so the slides and the code cannot drift apart. Two programs:

- `.` (ingestor): generator → bounded channel → batcher → bounded channel → worker pool → `_bulk`,
  with per-document retries, exponential backoff with jitter, and a dead-letter file.
- `./naive`: one document per request, sequentially. The baseline everybody starts with.

## Run

Start a single-node OpenSearch (security disabled, plain HTTP on 9200):

```bash
docker compose up -d
curl -s localhost:9200/_cluster/health | jq .status
```

Baseline, one document per request:

```bash
go run ./naive -docs 20000
```

The same bulk code with no parallelism (a "bulk script"):

```bash
go run . -docs 1000000 -workers 1
```

The ingestor:

```bash
go run . -docs 1000000 -workers 8 -batch 2000 -queue 4
```

Stress test: restart the node with a two-slot write queue so the cluster rejects
bulk requests with 429, then throw more workers at it than it can serve. Watch
`retried` climb while `failed` stays at zero:

```bash
WRITE_QUEUE_SIZE=2 docker compose up -d
go run . -docs 1000000 -workers 32 -batch 500
docker compose up -d   # back to the default queue
```

Rejected documents, if any, end up in `dead-letter.ndjson` with the status and reason.

The same stress test with a fixed attempt count instead of the retry window, which
is what most retry loops do and why the talk argues against it:

```bash
go run . -docs 1000000 -workers 32 -batch 500 -max-attempts 5
```

A 429 means the cluster is busy, and busy passes. Counting attempts gives up on
documents the cluster would have accepted a second later; they land in the
dead-letter file. `-retry-for` (the default, two minutes) waits it out instead.

Watch the write thread pool while it runs:

```bash
watch -n1 'curl -s "localhost:9200/_cat/thread_pool/write?v&h=name,active,queue,rejected,completed"'
```

Stop and wipe the data:

```bash
docker compose down -v
```

## Recorded runs

`runs/` holds the unedited output of four runs. Each one is the **median run of its
group**, not the best: every configuration was run five times (three for the
baseline) after a discarded warm-up, against one `opensearchproject/opensearch:3.8.0`
node in Docker on an Apple M1 Pro (Docker VM: 8 CPUs; 2 GiB heap, one shard, no
replicas, refresh off), with no other containers running. Documents are 159 bytes
of JSON, 194 on the wire once the bulk action line is added.

Stop every other container before you benchmark: they share the VM's CPUs with
OpenSearch. An earlier round of these runs, taken without that check, came out 16
to 36 percent slower and noisier than this one.

| File | Command | Median | Spread over the group |
|---|---|---|---|
| `naive.log` | `./naive -docs 20000` | 563 docs/s | 560 – 616 |
| `workers-1.log` | `-workers 1` | 46,222 docs/s | 41,673 – 47,531 |
| `workers-8.log` | `-workers 8 -batch 2000` | 176,112 docs/s | 141,063 – 177,493 |
| `stress-429.log` | `WRITE_QUEUE_SIZE=2`, `-workers 32 -batch 500` | 136,738 docs/s | 94,594 – 151,068 |

A laptop is not a measurement instrument: read the ratios between rows, not the
absolute numbers. What did not vary is the part that matters — across all 50 ingestor
runs on the default retry policy, every single one finished with `failed=0` and
exactly 1,000,000 documents in the index.

Two knobs, swept at 1,000,000 documents (workers five runs each, batch size three):

| Workers (batch 2000) | Median docs/s | | Docs per request (8 workers) | Median docs/s |
|---|---|---|---|---|
| 1 | 46,222 | | 500 | 152,239 |
| 2 | 80,447 | | 2,000 | 178,901 |
| 4 | 127,070 | | 5,000 | 159,562 |
| 8 | 176,112 | | 10,000 | 149,321 |
| 16 | 165,780 | | 25,000 | 121,412 |
| 32 | 151,544 | | | |

Workers scale close to linearly up to eight, the node's write thread count, and
peak there; sixteen and thirty-two are no faster (medians 6 and 14 percent lower,
with ranges that overlap eight's). Batch size has its best median at 2,000; 500,
5,000 and 10,000 medians are 10 to 17 percent lower, and 25,000 is a third lower.
Batch sizes had three runs each, so read that as a broad optimum, not a sharp peak.

The give-up rule, under the same two-slot write queue (five runs each): the
two-minute retry window dead-lettered nothing on any run; `-max-attempts 5`
dead-lettered 1,000 documents on one run in five; `-max-attempts 3` dead-lettered
4,000 to 7,500 on every run.

## Update: opensearch-go v5

This code was written against **opensearch-go v4.7.3**, the version used in the talk.
Two things changed in the client after it, and one thing did not.

**v4.7.0** added typed partial-failure errors (`PartialBulkError`, `PartialSearchError`,
`ShardFailureError`), but masked them off by default, so a rejected document still
produced no Go error. You can opt in on v4:

```go
client, _ := opensearchapi.NewClient(opensearchapi.Config{
    Client: opensearch.Config{Addresses: []string{addr}},
    Errors: new(errmask.Empty), // or: OPENSEARCH_GO_ERROR_MASK=empty
})
```

**v5.0.0** (22 September 2026) makes that the default. `Bulk` returns a
`*PartialBulkError` carrying `FailedItems []BulkRespItem` (each with its own `Status`
and `Error`) and `SucceededCount`, so the client finds the rejected documents for you:

```go
// v4.7.3, as in this repo: check the flag, walk every item yourself
resp, err := client.Bulk(ctx, opensearchapi.BulkReq{Index: index, Body: body})
if err != nil { return err }          // only "the request did not arrive"
if !resp.Errors { return nil }
for i, item := range resp.Items { /* item["index"].Status, batch[i] */ }

// v5: the rejected items arrive with the error
resp, err := client.Bulk(ctx, opensearchapi.BulkReq{Index: index, Body: body})
for _, sub := range opensearchapi.Errors(err) {
    if e, ok := sub.(*opensearchapi.PartialBulkError); ok {
        for _, item := range e.FailedItems { /* item.Status, item.Error */ }
    }
}
```

**What did not change**, and is still this repo's job:

- the client does not retry `429` for you (`RetryOnStatus` defaults to 502, 503, 504);
- `opensearchutil.BulkIndexer` still calls `OnFailure` with a **nil** error for
  per-document rejections, so the "HTTP 200 is not success" trap is still easy to hit;
- deciding *which* failures are worth retrying, *how long* to keep trying (this repo
  uses a 2-minute window rather than a fixed attempt count), and *where* the give-ups
  go (`dead-letter.ndjson`) is policy the client does not provide.

So v5 improves **detection**. The retry, backoff, give-up rule and dead-letter file in
`ingest.go`, `retry.go` and `deadletter.go` stay exactly as the talk describes them.
