---
theme: default
title: Building a High Throughput OpenSearch Ingestor using Go
titleTemplate: '%s'
favicon: /favicon.png
htmlAttrs:
  lang: en
# Social metadata lives here (Slidev emits it as Open Graph and Twitter tags).
# The plain <meta name="description"> is in index.html, the one tag Slidev has no key for.
seoMeta:
  ogTitle: Building a High Throughput OpenSearch Ingestor using Go
  ogDescription: Worker pool, bounded channels for backpressure, and per-document retries with a dead-letter file. 63 measured runs against OpenSearch 3.8; all 50 on the default retry policy were lossless. OpenSearchCon North America 2026, Rajiv Ranjan Singh.
  ogImage: https://iamrajiv.github.io/opensearchcon-north-america-2026/og.png
  ogUrl: https://iamrajiv.github.io/opensearchcon-north-america-2026/
  twitterCard: summary_large_image
  twitterSite: '@therajiv'
  twitterTitle: Building a High Throughput OpenSearch Ingestor using Go
  twitterDescription: Worker pool, bounded channels for backpressure, and per-document retries with a dead-letter file. 63 measured runs against OpenSearch 3.8; all 50 on the default retry policy were lossless.
  twitterImage: https://iamrajiv.github.io/opensearchcon-north-america-2026/og.png
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

<img src="/images/opensearchcon-north-america.svg" class="event-logo" alt="OpenSearchCon North America" />

# Building a High Throughput OpenSearch Ingestor using Go

<p class="meta">OpenSearchCon North America 2026, San Jose, USA, 22nd–24th September 2026</p>
<p class="meta"><strong>Rajiv Ranjan Singh</strong></p>

<!--
Hello everyone, and thank you for staying for the last session before lunch. I'm Rajiv. This talk is about one practical problem: getting a lot of data into OpenSearch fast, without overloading the cluster, and without losing a single document.
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
Here's the plan. First, where ingestion gets slow. Then why Go fits this problem. Then we build the ingestor step by step, and we finish with the numbers.

I won't run anything live. Everything ran on my laptop before the talk, and the real output is in the slides. The code is in the repository linked at the end.
-->

---
layout: two-cols
layoutClass: speaker
---

# Rajiv Ranjan Singh

<p class="meta">x.com/therajiv · github.com/iamrajiv</p>

- Software Engineer (JL3) at **A.P. Moller Maersk**, Platform Engineering, Bengaluru
- Graduated from **JSSATE, Bengaluru, India** in 2022
- **GSoC** 2022, **LFX Mentorship** 2021, **GSoD** 2020 & 2021
- Mentor in **GSoC** 2023 to 2026 with **Jenkins**
- Previously **Lummo**, **redBus**, and **Economize**

::right::

<div class="portrait"><img src="/images/rajiv.webp" alt="Rajiv Ranjan Singh" /></div>

<!--
A quick word about me. I'm a software engineer at A.P. Moller Maersk, the shipping and logistics company, in the platform engineering org in Bengaluru. I mostly write backend systems in Go. I've also been part of Google Summer of Code for a few years, and now I mentor there with Jenkins.
-->

---

# Where ingestion gets slow

<v-clicks>

- **One document per request**: one HTTP round trip per document, a few hundred to a thousand docs per second
- **A bulk script**: batches into `_bulk`, but sequentially, so the cluster idles while the client encodes and waits
- **A parallel script**: fast until the write queue fills, then `429 rejected_execution_exception`
- **The quiet failure**: HTTP 200 with `errors: true`, and nobody reads the items

</v-clicks>

<!--
Most of us start the same way, and we hit the same walls.

[click] Wall one: one request per document. Every document waits for its own trip over the network. On my laptop that was about 560 documents a second. A million documents would take half an hour.

[click] Wall two: you find the bulk API, which sends many documents in one request. Much better. But the script sends one bulk request at a time, so the cluster sits idle while it waits.

[click] Wall three: you add threads with no limit. It's fast for a minute, then the cluster answers 429, too many requests. That's not a bug. The cluster's queue is full, and it's asking you to slow down.

[click] And under all three, there's a quiet failure. The bulk API can answer 200 OK even when some documents inside were rejected. If you only check the status code, you lose data and never know.

These four walls are the three problems the rest of the talk solves: go fast, don't overload the cluster, and don't lose a document.
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
So why Go? Python or Java can do this too, but Go makes it short.

[click] A goroutine is a very cheap thread, and a channel is a queue between goroutines. Together they give you a worker pool in a few lines.

[click] A channel has a fixed size. When it's full, whoever sends to it waits. So when the cluster slows down, your program slows down with it. That's backpressure, and you get it for free.

[click] Because the queues have a fixed size, memory stays flat no matter how much data you load. And it builds to one binary that runs the same on your laptop, in CI, or in Kubernetes.

[click] OpenSearch also has an official Go client, opensearch-go, and that's what I use.
-->

---
clicks: 4
---

# One pipeline, two bounded channels, N workers

<Pipeline />

<!--
This is the whole design, left to right. A generator produces documents; in real life that's your files, Kafka, or a database. A batcher groups them into batches of 2,000. Then 8 workers take the batches and send them to OpenSearch.

If you don't write Go, don't worry. This is just a bounded queue and a fixed pool of workers, and it works the same way in Java or Python.

[click] Each worker sends one request at a time. So 8 workers means at most 8 requests being handled at once. That number is your throttle.

[click] Now the cluster slows down. The workers wait for responses, nobody takes new batches, and the queue in front of them fills up.

[click] When that queue is full, the batcher waits, and then the generator waits. The whole pipeline pauses with a fixed amount of memory, and it starts again by itself when the cluster catches up.

[click] A 429 does the same thing on a small scale. The worker sleeps before it retries, and a sleeping worker takes no new batches. The cluster sets the pace.
-->

---

# One connection per worker

<<< @/examples/ingestor/client.go#client {2-4|6-11|13-14}

<!--
PACE: about 5 minutes in.

Now the code, starting with the client. There's a hidden trap in the first lines.

Go's HTTP client keeps only 2 idle connections per server by default. With 8 workers, 6 of them would open a new connection on every request. So I raise the limit to one connection per worker.

[click] Then we create the OpenSearch client with that connection setup.

[click] RetryOnStatus makes the client retry the whole request on 502, 503 and 504, when the server is briefly unavailable. I leave 429 out on purpose. In a bulk load, a 429 usually comes back per document, inside a 200 response. We handle those ourselves, and I'll show you how.
-->

---

# Stable document IDs make retries safe

<<< @/examples/ingestor/bulk.go#encode {*|5|2-3,6}

<v-click>

<p class="note">Batch size: OpenSearch's tuning guide says start at <strong>5 to 15 MiB</strong> per request and raise it until throughput stops improving.</p>

</v-click>

<!--
This builds the bulk request. The format is simple: for each document, one line that says what to do, then one line with the document itself.

[click] Here's the important decision: we set our own document IDs. Imagine a request succeeds, but the response gets lost on the network. We retry. With our own ID, the retry just overwrites the same document. No duplicates. Doing it twice gives the same result as doing it once.

[click] A small detail: we build the request in one buffer instead of joining strings. When you send tens of megabytes a second, that saves real CPU.

[click] How big should a batch be? OpenSearch's guide says start at 5 to 15 MB. I measured on my setup, and 2,000 documents per request worked best. Very large batches of 25,000 were a third slower. Measure with your own data.
-->

---

# The worker pool is eleven lines

<<< @/examples/ingestor/pool.go#workers {*|3-4|5-7|10}

<!--
This is the worker pool. All of it, 11 lines.

[click] We start 8 goroutines (one per worker).

[click] Each one loops over the batches channel. It takes the next batch, or waits if there isn't one. No locks needed: the channel is the queue.

[click] The wait group waits until every worker has finished its last batch. And if you press Ctrl-C, each worker finishes the batch in its hands and stops cleanly.
-->

---

# HTTP 200 is not success

<<< @/examples/ingestor/bulk.go#send {1-5|6-8|9-14|15-21}

<!--
Now the part most scripts get wrong: reading the response. There are three cases.

First, the request itself fails, like a timeout or a dropped connection. We return the error, and the whole batch is retried.

[click] Second, the response says errors is false. Every document was saved. Nothing more to do.

[click] Third, errors is true. Now we check each item. The results come back in the same order we sent the documents: the first result is for our first document, and so on.

[click] Every failed document goes into a list with its status and its reason. 429 means the cluster was busy, so it's worth retrying. 400 means the document itself is bad, so retrying won't help. And note: the Go client does not turn these failures into an error. You have to look.
-->

---

# Retries without losing a document

<<< @/examples/ingestor/ingest.go#retry {3-4,9|10-16|18-21}

<!--
This is the retry loop for one batch. We send it, and whatever comes back rejected becomes the new batch. 2,000 go out, 30 are rejected, and only those 30 go out again.

[click] Each rejected document goes one of two ways. If it's worth retrying, and we're still inside the retry window of 2 minutes, it stays for another round. Otherwise it goes to the dead-letter file: a plain file with the document and the reason. Nothing is silently dropped.

[click] Before the next round, the worker sleeps a little, a bit longer each time, plus some randomness. And a sleeping worker isn't taking new batches, so this also slows the whole pipeline down.
-->

---

# Retry 429 and 503, dead-letter the rest

<<< @/examples/ingestor/retry.go#backoff

- Base **200 ms**, cap **10 s**, full jitter so workers do not retry in lockstep

<v-clicks>

- **429** and **503**: the cluster says *not now*, retry for as long as you can afford to wait
- **400, 404, 409**: the cluster says *not like that*, straight to the dead-letter file<br><span class="dim">409 is success instead, if you index with <code>create</code> for de-duplication</span>
- Whole-request errors retry the entire batch; stable IDs make that safe

</v-clicks>

<!--
Here's the wait between retries. It starts at 200 ms, doubles each time, and never goes above 10 seconds. The random part is called jitter. It stops all the workers from retrying at the same moment.

[click] 429 and 503 mean "not right now". Retry them.

[click] 400, 404 and 409 mean "not like that". Retrying won't help, so they go straight to the dead-letter file.

[click] And if the whole request fails, like a timeout, we retry the whole batch. Our own document IDs make that safe.
-->

---

# Turn off refresh and replicas while loading

<<< @/examples/ingestor/setup.go#prepare

<v-clicks>

- `refresh_interval: -1` while loading, back to `1s` when done
- `number_of_replicas: 0` while loading, back to your normal count when done
- Bulk request size: start at **5 to 15 MiB**, raise until throughput flattens
- Watch `_cat/thread_pool/write` for `queue` and `rejected`

</v-clicks>

<!--
The client is only half the job. The other half is not giving the cluster extra work during the load. This code creates the index with two settings changed.

[click] Refresh is how OpenSearch makes new documents searchable, normally every second. Nobody is searching during a backfill, so we turn it off, and at the end we turn it back on and count the documents.

[click] A replica is a copy of the data on another machine, and every copy means indexing each document again. So we set replicas to zero during the load and add them back afterwards. The risk: if a machine dies mid-load, you run the load again.

[click] Keep bulk requests a sensible size: start around 5 to 15 MB, and measure.

[click] And while it runs, watch the write thread pool. The queue and the rejected count tell you if you're pushing too hard.
-->

---

# The bulk API is 80x. The worker pool is another 3.8x.

One OpenSearch 3.8 node in Docker (8 CPUs, 2 GiB heap), 1,000,000 synthetic events of 194 bytes on the wire. Warm-up discarded, every configuration run five times.

<table>
  <thead>
    <tr><th>Loader</th><th class="numeric">Docs/s</th><th class="numeric">Spread over 5 runs</th><th class="numeric">MB/s</th><th>Time for 1M</th></tr>
  </thead>
  <tbody>
    <tr><td>One document per request</td><td class="numeric">563</td><td class="numeric">560 – 616</td><td class="numeric">0.1</td><td>~30 min (measured on 20k)</td></tr>
    <tr v-click><td>Bulk, 1 worker, 2000 per request</td><td class="numeric">46,222</td><td class="numeric">41,673 – 47,531</td><td class="numeric">9.0</td><td>21.6 s</td></tr>
    <tr class="lead" v-click><td>Bulk, 8 workers, 2000 per request</td><td class="numeric">176,112</td><td class="numeric">141,063 – 177,493</td><td class="numeric">34.2</td><td>5.7 s</td></tr>
    <tr v-click><td>32 workers, write queue shrunk to 2</td><td class="numeric">136,738</td><td class="numeric">94,594 – 151,068</td><td class="numeric">31.8</td><td>7.3 s, 0 dead letters</td></tr>
  </tbody>
</table>

<v-click>

<p class="note">Median of five runs each, three for the baseline. Across all <strong>50</strong> ingestor runs on the default retry policy, every one ended with <strong>failed=0</strong> and exactly <strong>1,000,000</strong> documents in the index.</p>

</v-click>

<!--
PACE: about 11 minutes in.

Now the numbers. Everything ran on my laptop: one OpenSearch node in Docker, and a million small log events. I ran every setup 5 times and show the middle result and the range. So please read the ratios, not the exact numbers.

[click] Just switching to the bulk API, with one worker: 46,222 documents a second. 80 times faster, from one change.

[click] 8 workers: 176,112 a second. A million documents in under 6 seconds. Nearly 4 times the single worker.

[click] The last row is a stress test, with the cluster overloaded on purpose. It's slower, but look at the last column: zero documents lost.

[click] And this is the result that matters most. In all 50 runs, every single one ended with 0 failures and exactly 1,000,000 documents in the index.
-->

---

# Workers are the throttle; the cluster is the ceiling

<table>
  <thead>
    <tr><th class="numeric">Workers</th><th class="numeric">Docs/s</th><th class="numeric">vs 1 worker</th><th class="numeric">Spread</th><th class="barcell">&nbsp;</th></tr>
  </thead>
  <tbody>
    <tr><td class="numeric">1</td><td class="numeric">46,222</td><td class="numeric">1.0x</td><td class="numeric">1.1x</td><td class="barcell"><span class="bar" style="--w:26%"></span></td></tr>
    <tr><td class="numeric">2</td><td class="numeric">80,447</td><td class="numeric">1.7x</td><td class="numeric">1.1x</td><td class="barcell"><span class="bar" style="--w:46%"></span></td></tr>
    <tr><td class="numeric">4</td><td class="numeric">127,070</td><td class="numeric">2.7x</td><td class="numeric">1.0x</td><td class="barcell"><span class="bar" style="--w:72%"></span></td></tr>
    <tr class="lead"><td class="numeric">8</td><td class="numeric">176,112</td><td class="numeric">3.8x</td><td class="numeric">1.3x</td><td class="barcell"><span class="bar lead" style="--w:100%"></span></td></tr>
    <tr><td class="numeric">16</td><td class="numeric">165,780</td><td class="numeric">3.6x</td><td class="numeric">1.5x</td><td class="barcell"><span class="bar" style="--w:94%"></span></td></tr>
    <tr><td class="numeric">32</td><td class="numeric">151,544</td><td class="numeric">3.3x</td><td class="numeric">1.2x</td><td class="barcell"><span class="bar" style="--w:86%"></span></td></tr>
  </tbody>
</table>

<v-click>

<p class="note">Throughput peaks at <strong>8</strong> workers, and the node has <strong>8</strong> write threads. Past that, extra requests only wait in the write queue: <strong>16</strong> and <strong>32</strong> were no faster than 8. Raise workers until the curve flattens, then stop.</p>

</v-click>

<!--
This slide answers one question: how many workers should I use? Same data, only the worker count changes.

1, 2, 4, 8 workers: each step adds a lot, from 46,222 to 176,112 a second. 8 is the peak, and that's not a coincidence. The node has 8 CPUs, so it has 8 indexing threads.

[click] After 8, more workers don't help. 16 and 32 were no faster, because the extra requests just wait in the cluster's queue. So add workers until the numbers stop going up, then stop. A good starting point is the number of indexing threads in your cluster.
-->

---

# Under a two-slot write queue, nothing was lost

<RunLog src="stress-429" />

<p class="note">Write queue of <strong>2</strong>, <strong>32</strong> workers: documents rejected with 429 were re-sent <strong>197,504</strong> times, and <code>_cat/thread_pool/write</code> counted <strong>397</strong> rejected tasks. Every rejected document was retried, the dead-letter file stayed empty, and the index ended with exactly <strong>1,000,000</strong> documents &mdash; on this run and on the four others.</p>

<v-click>

<p class="note">Same test, five runs each, only the give-up rule changed. A <strong>time window</strong> lost nothing, ever. <strong>5 fixed attempts</strong> dead-lettered up to <strong>1,000</strong> documents. <strong>3 fixed attempts</strong> dead-lettered <strong>4,000 to 7,500</strong>, on every run. A 429 is the cluster asking you to wait, not to give up.</p>

</v-click>

<!--
The last test is the hardest one. I shrank the cluster's write queue to just 2 slots and pointed 32 workers at it, so the cluster answers 429 all the time.

Watch the retried count climb, and watch failed: 0 on every line. At the end, 1,000,000 documents, counted from the index itself. I ran this 5 times, and all 5 ended the same way.

[click] Now the mistake I made first. My first version gave each document a fixed number of attempts, like most retry code does. With 5 attempts, one run lost 1,000 documents. With 3, every run lost thousands. With a time window, nothing was ever lost. A 429 means "wait", not "give up".
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
PACE: about 14 minutes in.

To wrap up.

[click] Use bulk requests, sent in parallel by a fixed number of workers.

[click] Give every queue a fixed size, and let the waiting slow you down.

[click] 200 OK is not success. Check every document.

[click] Retry only what was rejected, with growing, random waits.

[click] Use your own document IDs, and keep a dead-letter file.

[click] Turn off refresh and replicas while loading, and turn them back on after.

[click] And if you don't want to write the worker pool yourself, the client's BulkIndexer does it for you. But it won't retry 429s, so reading the response is still your job.
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
Everything is in the repository: the code, the slides, and the docker compose file, so you can run it yourself. Scan the QR code, or find me on GitHub as iamrajiv.
-->

---
layout: cover
class: contrast
---

# Thank you

<p class="meta contact name"><strong>Rajiv Ranjan Singh</strong></p>
<p class="meta contact link">x.com/therajiv</p>
<p class="meta contact link">github.com/iamrajiv</p>

<img src="/images/opensearchcon-north-america.svg" class="event-logo closing" alt="OpenSearchCon North America" />

<figure class="feedback">
  <img src="/images/feedback-qr.png" alt="QR code to rate this session on Sessionize" />
  <figcaption>Rate this session</figcaption>
</figure>

<!--
Thank you very much. If you have a minute, the QR code on the right opens the feedback form. I'm happy to take questions now, and I'll be around during lunch.
-->
