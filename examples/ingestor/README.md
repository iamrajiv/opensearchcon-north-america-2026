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

Watch the write thread pool while it runs:

```bash
watch -n1 'curl -s "localhost:9200/_cat/thread_pool/write?v&h=name,active,queue,rejected,completed"'
```

Stop and wipe the data:

```bash
docker compose down -v
```

## Recorded runs

`runs/` holds the unedited output of the four runs behind the numbers in the
talk, all from one laptop against a single OpenSearch 3.8.0 node in Docker
(8 CPUs, 2 GB heap), 1,000,000 synthetic events of about 200 bytes:

| File | Command | Result |
|---|---|---|
| `naive.log` | `go run ./naive -docs 20000` | 411 docs/s |
| `workers-1.log` | `go run . -docs 1000000 -workers 1` | 42,536 docs/s |
| `workers-8.log` | `go run . -docs 1000000 -workers 8` | 150,098 docs/s |
| `stress-429.log` | `WRITE_QUEUE_SIZE=2`, `-workers 32 -batch 500` | 123,228 docs/s, 1,019 bulk requests rejected, 0 dead letters |
