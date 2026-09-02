---
theme: default
title: Building a High Throughput OpenSearch Ingestor using Go
titleTemplate: '%s'
info: |
  ## Building a High Throughput OpenSearch Ingestor using Go
  OpenSearchCon North America 2026 · San Jose · Rajiv Ranjan Singh
author: Rajiv Ranjan Singh
lineNumbers: false
colorSchema: light
aspectRatio: 16/9
canvasWidth: 980
fonts:
  provider: none
  sans: Geist
  mono: Geist Mono
drawings:
  persist: false
exportFilename: slides
layout: cover
class: contrast
---

# Building a High Throughput OpenSearch Ingestor using Go

<p class="meta"><strong>Rajiv Ranjan Singh</strong> · Software Engineer, A.P. Moller Maersk</p>
<p class="meta">OpenSearchCon North America 2026 · Operating OpenSearch · San Jose, Thursday, September 24</p>

<!--
Hello everyone, and thank you for staying for the last session before lunch. My name is Rajiv, and for the next twenty minutes I want to talk about one very practical problem: getting a large amount of data into OpenSearch quickly, without overloading the cluster, and without losing any of it on the way.

Most of us have lived this. You need to backfill an index or replay a few days of logs, so you write a small script. It works on ten thousand documents. Then you point it at a hundred million, and either it takes all night, or the cluster starts answering 429 and the script quietly drops data.
-->

---

# Agenda

- Where ingestion gets slow
- Why Go for an ingestor
- The pipeline: worker pool, bounded channels, bulk requests
- Backpressure: not overloading the cluster
- Retries without losing a document
- Cluster settings that matter during a load
- Results and key takeaways

<!--
Here is the plan. First, where ingestion actually gets slow, because the wall is in the same place for everyone. Then why I reach for Go for this. Then we build the ingestor piece by piece: the pipeline, the worker pool, the bulk requests, backpressure to keep the cluster healthy, and retries to keep your data safe. Then a few cluster settings, and the numbers from my runs.

One thing up front: this is a twenty minute slot, so I will not run anything live. Everything was run on my laptop before the talk and the real output is in the slides. The code and a docker compose file are in the repository on the last slide, so you can reproduce all of it tonight.
-->

---
layout: image-right
image: /images/rajiv.png
backgroundSize: contain
---

# Rajiv Ranjan Singh

<p class="meta">x.com/therajiv · github.com/iamrajiv</p>

- Software Engineer (JL3) at **A.P. Moller Maersk**, Platform Engineering, Bengaluru
- Graduated from **JSSATE, Bengaluru, India**
- **GSoC** 2022, **LFX Mentorship** 2021, **GSoD** 2020 & 2021
- Mentor in **GSoC** 2023 to 2026 with **Jenkins**
- Previously **Lummo**, **redBus**, and **Economize**

<!--
A quick word about me. I am Rajiv, a software engineer at A.P. Moller Maersk, the shipping company, in the platform engineering team in Bengaluru. My team builds the internal developer platform that other Maersk engineers deploy on, and I mostly write backend systems in Go.

Outside work I have been part of Google Summer of Code, LFX Mentorship, and Season of Docs, and for the past four years I have mentored students in Google Summer of Code with Jenkins. Most of what follows comes from moving logs and events into search clusters in volume, and getting it wrong a few times first.
-->

---

# Where ingestion gets slow

<v-clicks>

- **One document per request**: one HTTP round trip per document, a few hundred to a thousand docs per second
- **A bulk script**: batches into `_bulk`, but sequentially, so the cluster idles while the client encodes and waits
- **A parallel script**: fast until the write queue fills, then `429 es_rejected_execution_exception`
- **The quiet failure**: HTTP 200 with `errors: true`, and nobody reads the items

</v-clicks>

<!--
Let me start with the wall, because everybody hits the same one.

[click] Stage one is the first loader everyone writes: one index request per document. Every document pays a full HTTP round trip, connect, send, wait, so your speed is limited by network latency, not by the cluster. On my laptop, against a cluster on the same machine, that was about four hundred documents a second.

[click] Stage two is discovering the bulk API. A bulk request is one HTTP request carrying many documents, say two thousand instead of one. But most scripts still send one bulk request at a time, and while your program encodes the next batch and waits, the cluster sits idle.

[click] Stage three is adding threads without a limit. Fast for a minute, then the cluster answers 429. That status means too many requests, and OpenSearch attaches es_rejected_execution_exception, which in plain words means: the queue of indexing work on the data node is full. That is not a bug. That is the cluster protecting itself from you.

[click] And under all three there is a quiet failure. The bulk API returns HTTP 200 even when half the documents inside failed. The failures are reported inside the response body, one by one. If your code only checks the status code, you have lost data and you do not know it.
-->

---

# Why Go for an ingestor

<v-clicks>

- Goroutines and channels give you a **worker pool with a bounded queue** in a few lines
- A blocked channel send **is** backpressure, with no extra machinery
- Fixed memory in flight, and one static binary that runs the same on a laptop, in CI, and in a Kubernetes job
- `opensearch-go` is the official client, with a Bulk API and a helper indexer

</v-clicks>

<!--
So why Go? Python and Java can do this too, but the shape of the problem fits Go unusually well.

[click] An ingestor is a pipeline: something produces documents, something groups them into batches, something sends the batches. Those three should run at the same time, with a limit on how much is in flight. Go has the two building blocks in the language. A goroutine is a very cheap thread the runtime manages for you; you can run thousands. A channel is a typed queue: one goroutine puts items in, another takes them out.

[click] And channels have the property that does most of the work in this talk. A channel has a fixed capacity, and when it is full, whoever tries to put something in simply waits. So when the cluster slows down, the workers wait on the cluster, the channel behind them fills, and everything upstream slows down to match. That is backpressure, and you get it without writing a rate limiter.

[click] Two consequences. Bounded channels mean the program holds only a handful of batches in memory, whatever the input size. And Go builds one static binary, so what you tested on your laptop is what runs as the migration job.

[click] Last piece: OpenSearch has an official Go client, opensearch-go, at version four, with the bulk API and a helper called BulkIndexer. If you are on a managed service, Data Prepper and OpenSearch Ingestion pipelines exist too. This talk is about writing the client yourself, so I will build the pipeline by hand and say at the end where the helper fits.
-->

---
clicks: 4
---

# One pipeline, two bounded channels, N workers

<Pipeline />

<!--
This is the whole architecture, left to right. One goroutine generates documents. In real life that is your source: a file, a Kafka topic, a database, Parquet files in object storage. Here it generates synthetic log events of about two hundred bytes. The batcher groups them into slices of two thousand, or sends what it has after half a second so a slow source never leaves a partial batch waiting. Batches go into the second channel, and N workers take them, encode each into a bulk request, and send it.

[click] When it runs, documents flow left to right. Each worker has exactly one request in flight at a time, so N workers means N concurrent bulk requests, never more. N is your throttle.

[click] Now OpenSearch slows down. Responses take longer, every worker is stuck waiting, nobody takes batches out of the second channel, and it fills up to its capacity and stops there.

[click] The batcher tries to add one more batch and has to wait. It stops reading documents, the first channel fills, and the generator waits too. The whole pipeline pauses, holding a fixed amount of memory, and resumes on its own when the cluster catches up. The two channel capacities are the entire backpressure policy.

[click] A 429 is the same story in miniature. A worker gets rejected documents, backs off, and sleeps. A sleeping worker is not taking batches, so the slowdown spreads upstream the same way. The cluster sets the pace, and the channels carry that signal back. Three knobs: batch size, number of workers, queue capacity.
-->

---

# One connection per worker

<<< @/examples/ingestor/client.go#client {2-4|6-11|13-14}

<!--
Now some code, starting with the client, because the first lines hide a mistake that can cost half your throughput.

An HTTP client keeps a pool of open connections so it can reuse them. Go's default transport keeps only two idle connections per host. With sixteen workers, fourteen of them open a new connection on every request, and with TLS that includes a full handshake each time. So I clone the default transport and raise the idle connection limit to the number of workers. One connection per worker.

[click] Then the client itself. In version four you create an opensearchapi client and pass the lower level configuration inside: addresses, optional username and password, and our transport.

[click] Look at RetryOnStatus. It retries the whole request on 502, 503 and 504, all forms of the server being temporarily unavailable. I left 429 out on purpose. During a bulk load a 429 almost never arrives as the status of the whole request. It arrives per document, inside a response whose status is 200. We handle that ourselves, one document at a time.
-->

---

# Stable document IDs make retries safe

<<< @/examples/ingestor/bulk.go#encode {*|5|2-3,6}

<v-click>

<p class="note">Batch size: OpenSearch's tuning guide says start at <strong>5 to 15 MB</strong> per request and raise it until throughput stops improving.</p>

</v-click>

<!--
This builds the bulk request body. The bulk API wants newline delimited JSON: for every document, one action line saying what to do, then one line with the document. The action is index, and it carries the document ID.

[click] That is the first important decision: we set our own IDs instead of letting OpenSearch generate them. If a document was indexed but the response got lost on the network, our retry sends it again, and with our own ID the second index simply overwrites the first. That property is called idempotence: doing it twice has the same result as once. If your data has no stable ID, use the create action and treat a 409 conflict as success.

[click] A smaller detail: we encode straight into one buffer, no string building. At tens of megabytes of JSON a second, encoding shows up in your CPU profile.

[click] On batch size, OpenSearch's tuning guide says start between five and fifteen megabytes per request and raise it until throughput stops improving. Two thousand of these events is under a megabyte, so there is room. Measure with your documents, not mine.
-->

---

# The worker pool is eleven lines

<<< @/examples/ingestor/pool.go#workers {*|3-4|5-7|10}

<!--
This is the worker pool. All of it, eleven lines.

[click] We start N goroutines in a loop. That is the entire cost of a worker in Go.

[click] Each one ranges over the batches channel. Range means: take the next item, and if there is none yet, wait. So batches go to whichever worker is free, and when the channel is closed and empty the loop ends by itself. No mutex, no semaphore. The channel is the queue and the goroutines are the pool.

[click] The wait group makes main wait until every worker has finished its last batch, so the throughput we print is honest. And because workers get the parent context, control C cancels the generator, the channels drain, and each worker finishes the batch in its hands. Graceful shutdown for free.
-->

---

# HTTP 200 is not success

<<< @/examples/ingestor/bulk.go#send {1-5|6-8|9-14|15-21}

<!--
Now the part almost every script gets wrong: reading the response. Three cases.

First, the call itself returns an error: a connection reset, a timeout, a 5xx. We return it and the caller retries the entire batch.

[click] Second, the request succeeded and the errors flag is false. Every document was accepted. Fast path.

[click] Third, errors is true, so we walk the items. They come back in the same order we sent the documents, so item i is document i in our batch. Each item has its own status and, if it failed, an error with a type and a reason.

[click] Anything with an error or a status of three hundred or higher goes into a failed list with the original document, its status, and the reason. The status decides what happens next: 429 means the node was busy, worth trying again; 400 means the document itself is wrong and will fail the same way again. One detail about the version four client: by default it does not turn these failures into a Go error. You get a normal response with errors set to true. So you have to look.
-->

---

# Retries without losing a document

<<< @/examples/ingestor/ingest.go#retry {3-4,9|10-16|18-21}

<!--
This is a worker's job for one batch, and it is the retry loop. We send the batch. Whatever came back rejected becomes the new batch, and only that. Two thousand went out, thirty were rejected, thirty go out again. Nothing accepted is ever sent twice.

[click] Each rejected document is sorted into one of two places. If its status is retryable and we are still inside the retry window, two minutes by default, it stays for another round. Otherwise it goes to the dead letter file: plain text, one JSON line per document with the status and the reason. Nothing is silently dropped. Fix the cause later and replay the file.

[click] If anything is left, we sleep with a delay that grows on every attempt plus a random amount called jitter, and go around again. Jitter matters: if eight workers get a 429 at the same moment and retry after the same delay, they hit the cluster together again. A random spread breaks that. And while a worker sleeps it is not taking batches, so the backoff is also backpressure. One mechanism, two sides.
-->

---

# Retry 429 and 503, dead-letter the rest

<<< @/examples/ingestor/retry.go#backoff

- Base **200 ms**, cap **10 s**, full jitter so workers do not retry in lockstep

<v-clicks>

- **429** and **503**: the cluster says *not now*, retry for as long as you can afford to wait
- **400, 404, 409**: the cluster says *not like that*, straight to the dead-letter file
- Whole-request errors retry the entire batch; stable IDs make that safe

</v-clicks>

<!--
And here is the backoff itself. It starts at two hundred milliseconds, doubles on every attempt, is capped at ten seconds, and adds a random amount so workers do not retry in lockstep.

[click] 429 and 503 mean not right now: retry them for as long as you can afford to wait.

[click] 400, 404 and 409 mean not like that: retrying is noise, so they go straight to the dead letter file with the reason attached.

[click] And whole request errors, a timeout or a reset, retry the entire batch. The stable IDs from earlier are what make that safe.
-->

---

# Turn off refresh and replicas while loading

<<< @/examples/ingestor/setup.go#prepare

<v-clicks>

- `refresh_interval: -1` while loading, back to `1s` when done
- `number_of_replicas: 0` while loading, back to your normal count when done
- Bulk request size: start at **5 to 15 MB**, raise until throughput flattens
- Watch `_cat/thread_pool/write` for `queue` and `rejected`

</v-clicks>

<!--
The client is half the story. The other half is not asking the cluster to do unnecessary work while you load. This function creates the index with two settings changed for the duration of the load.

[click] The first is the refresh interval. By default OpenSearch makes new documents searchable once a second, and to do that it writes a new segment file every second and merges them later. During a backfill nobody is searching yet, so that work is wasted. Minus one turns it off. When the load is done you set it back to one second and refresh once.

[click] The second is replicas. A replica is a copy of a shard on another node. Every document is indexed again on every replica, so with one replica the cluster does the indexing work twice. Set replicas to zero for the load and let the cluster copy the finished shards afterwards. The trade-off is real: lose a node during the load and you reload. For a rerunnable backfill, fine.

[click] The finish function restores the refresh interval, refreshes once, and counts the documents so we can prove nothing was lost.

[click] And keep the write thread pool statistics on a screen. Each data node has a fixed number of indexing threads and a queue in front of them. The queue depth and the rejected count tell you whether you are pushing at the right level.
-->

---

# Four runs, one laptop

**One OpenSearch node, security disabled for the demo:**

```bash
docker compose up -d
```

<v-click>

**Baseline, one document per request:**

```bash
go run ./naive -docs 20000
```

</v-click>

<v-click>

**Same bulk code, one worker, then eight:**

```bash
go run . -docs 1000000 -workers 1
go run . -docs 1000000 -workers 8 -batch 2000 -queue 4
```

</v-click>

<v-click>

**Stress test, write queue shrunk to two slots:**

```bash
WRITE_QUEUE_SIZE=2 docker compose up -d && go run . -docs 1000000 -workers 32 -batch 500
```

</v-click>

<!--
These are the four runs I did before the talk, all on my laptop, and the next slides show their real output. The compose file starts one OpenSearch node with security disabled, so the demo talks plain HTTP with no credentials. Do not copy that to production.

[click] The naive loader first: twenty thousand documents, one request each, kept small because it takes a while.

[click] Then the same ingestor twice: one worker, which behaves like the sequential bulk script most people have, and eight workers, the pipeline we just walked through. A million events each.

[click] Last, the stress test: the node restarted with its write queue shrunk to two slots, which guarantees 429s, and thirty two workers pointed at it, to watch retried climb while failed stays at zero.
-->

---

# The eight-worker run, second by second

<<< @/examples/ingestor/runs/workers-8.log {*|7-8}

<p class="note">One line per second: documents indexed so far, the rate in that second, bulk requests sent, and the retried and failed counters. The final count comes from the index itself.</p>

<!--
This is the real output of the eight worker run. One line per second: documents indexed so far, the rate in that second, bulk requests sent, and the retried and failed counters. The rate settles between one hundred and twenty and one hundred and eighty thousand a second.

[click] The summary: one million indexed, zero failed, five hundred and five bulk requests, six point seven seconds. And that final count comes from asking the index itself, not from our counters.
-->

---

# Under a two-slot write queue, nothing was lost

<<< @/examples/ingestor/runs/stress-429.log {*|9-10}

<v-click>

<p class="note">Write queue of <strong>2</strong>, <strong>32</strong> workers: the cluster rejected <strong>1,019</strong> bulk requests with 429. Every rejected document was retried, and the index ended with exactly <strong>1,000,000</strong> documents.</p>

</v-click>

<!--
And the stress run. Same code, but the write queue has two slots and there are thirty two workers, so the cluster rejects bulk requests constantly. Look at the retried column climb past two hundred thousand, and look at the failed column: zero on every line.

[click] The summary again: one million indexed, zero failed, confirmed by the index count. The cluster rejected over a thousand bulk requests, and the run still finished in eight seconds, less than twenty percent slower than the healthy run.
-->

---

# Eight workers index 150,000 documents a second

Same laptop, one OpenSearch 3.8 node in Docker (8 CPUs, 2 GB heap), 1,000,000 synthetic events of ~200 bytes.

<table>
  <thead>
    <tr><th>Loader</th><th class="numeric">Docs/s</th><th class="numeric">MB/s</th><th>Time for 1M docs</th></tr>
  </thead>
  <tbody>
    <tr><td>One document per request</td><td class="numeric">411</td><td class="numeric">0.1</td><td>~40 min (measured on 20k)</td></tr>
    <tr v-click><td>Bulk, 1 worker, 2000 per request</td><td class="numeric">42,500</td><td class="numeric">8.3</td><td>23.5 s</td></tr>
    <tr class="lead" v-click><td>Bulk, 8 workers, 2000 per request</td><td class="numeric">150,000</td><td class="numeric">29.1</td><td>6.7 s</td></tr>
    <tr v-click><td>32 workers, write queue shrunk to 2</td><td class="numeric">123,000</td><td class="numeric">29.3</td><td>8.1 s, 0 dead letters</td></tr>
  </tbody>
</table>

<v-click>

<p class="note">With a fixed <strong>5 attempts</strong> instead of a time window, the same stress run dead-lettered <strong>4,500</strong> documents. A 429 is the cluster asking you to wait, not to give up.</p>

</v-click>

<!--
The numbers side by side. One laptop, one node in Docker, so read the ratios, not the absolute values. One document per request: four hundred and eleven a second. A million would take forty minutes.

[click] The same bulk code with one worker: forty two thousand a second, a hundred times faster. That is the bulk API alone, where most teams stop.

[click] Eight workers: a hundred and fifty thousand a second, about thirty megabytes of JSON a second, a million documents in under seven seconds. Not eight times faster, because the node has eight CPUs and is doing the real work, but three and a half times for one flag is a good deal.

[click] The stress row: throughput dropped to a hundred and twenty three thousand, the index still holds exactly one million documents, and the dead letter file is empty.

[click] One thing I learned preparing this. My first version gave each document five attempts. Under the same stress test it dead lettered four and a half thousand documents. A time window fixed it. A 429 is the cluster asking you to wait, not to give up.
-->

---

# Key takeaways

<v-clicks>

- **Bulk, in parallel, with a limit**: workers are your throttle, channels are your queue
- **Bound every channel** and let blocking be your backpressure
- **HTTP 200 is not success**: read `errors` and every item's status
- **Retry only what was rejected**, with exponential backoff and jitter
- **Stable document IDs** make retries idempotent; a dead-letter file makes them lossless
- **Turn off refresh and replicas** during the load, turn them back on after
- `opensearchutil.BulkIndexer` gives you the pool and the batching; the response handling is still yours

</v-clicks>

<!--
Pulling it together.

[click] Bulk requests, in parallel, with a fixed number of workers. Workers are your throttle, channels are your queue.

[click] Bound every channel and let the blocking be your backpressure. You do not need a rate limiter.

[click] A 200 from the bulk API means the request was understood, not that your documents were indexed. Read the errors flag and every item.

[click] Retry only what was rejected, with a delay that grows and some randomness.

[click] Stable IDs make it safe to retry; a dead letter file makes it safe to stop.

[click] Refresh and replicas off for the load, back on afterwards.

[click] And if you would rather not write the pool, the official client's BulkIndexer does the pool and the batching. It does not retry 429 on its own, and its failure callback gets a nil error for per document rejections, so you still read the status. The response handling is always yours.
-->

---
layout: two-cols
---

# References

- [github.com/iamrajiv/opensearchcon-north-america-2026](https://github.com/iamrajiv/opensearchcon-north-america-2026)
- [github.com/opensearch-project/opensearch-go](https://github.com/opensearch-project/opensearch-go)
- [Bulk API reference](https://docs.opensearch.org/latest/api-reference/document-apis/bulk/)
- [Tuning your cluster for indexing speed](https://docs.opensearch.org/latest/tuning-your-cluster/performance/)
- [Thread pool settings](https://docs.opensearch.org/latest/install-and-configure/configuring-opensearch/thread-pool-settings/)
- [HTTP 429 and es_rejected_execution_exception](https://opensearch.org/blog/error-logs/error-log-http-429-too-many-requests-and-esrejectedexecutionexception-overloaded-cluster/)

::right::

<div class="flex flex-col items-center justify-center h-full">
  <img src="/images/qr.png" class="w-52" alt="QR code to the repository" />
  <p class="meta mt-3">slides, code, compose file, notes</p>
</div>

<!--
All the code, the slides, the compose file, and these notes are in the repository. Scan the QR code or find me on GitHub as iamrajiv. The bulk API reference, the indexing performance page, and the thread pool settings are the three pages I keep open, and the blog post on 429 explains what the cluster is telling you.
-->

---
layout: cover
class: contrast
---

# Thank you

<p class="meta"><strong>Rajiv Ranjan Singh</strong> · x.com/therajiv · github.com/iamrajiv</p>
<p class="meta">github.com/iamrajiv/opensearchcon-north-america-2026</p>

<!--
Thank you very much. I am happy to take questions now, and I will be around during lunch if you want to talk about your own ingestion setup.
-->
