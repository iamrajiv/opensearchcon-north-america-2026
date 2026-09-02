# Building a High Throughput OpenSearch Ingestor using Go

OpenSearchCon North America 2026, San Jose, California, USA, 24th September 2026

- Rajiv Ranjan Singh

## About

Ingesting large datasets into OpenSearch gets slow without the right client
architecture. This talk builds an ingestion tool from scratch in Go on the
`opensearch-go` client: a worker pool sending bulk requests in parallel, bounded
channels for backpressure so the cluster is never overloaded, and per-document
retries with backoff and a dead-letter file so nothing is lost.

Thursday, 24 September 2026, 12:10 to 12:30 PM PDT, San Jose Ballroom Salon III-IV.
Track: Operating OpenSearch.

## Slides

[`slides.md`](slides.md), built with [Slidev](https://sli.dev). Typeset in Geist,
copied into `public/fonts` from the `geist` npm package (1.7.2, SIL Open Font
License) so the deck renders the same offline and on GitHub Pages.

```bash
npm install
npm run dev         # audience view at http://localhost:3030
npm run presenter   # presenter view with notes at http://localhost:3030/presenter
```

Open the presenter view on the laptop screen and the audience view on the
projector; they stay in sync. The notes pane follows the `[click]` markers, so
each advance highlights the next paragraph of the script.

Export the PDF that Sessionize asks for (Slidev drives the bundled
`playwright-chromium`):

```bash
npm run export        # slides.pdf
npm run export:pptx   # slides.pptx, speaker notes included
```

Page metadata (title, favicon, Open Graph and Twitter tags, `og.png`) is declared
in the headmatter of `slides.md`; the plain description tag sits in `index.html`
because Slidev has no headmatter key for it.

The deck is published to GitHub Pages by
[`.github/workflows/deploy.yml`](.github/workflows/deploy.yml) on every push to
`main`. Enable Pages once in the repository settings with the source set to
GitHub Actions.

## Demo

[`examples/ingestor`](examples/ingestor) has the ingestor, the naive baseline,
a `docker-compose.yml` for a single OpenSearch node, and the recorded outputs
in `runs/` that the slides use. See its
[README](examples/ingestor/README.md) to run it.

## License

[MIT](LICENSE). The OpenSearchCon North America mark in `public/images` belongs to
the OpenSearch Software Foundation and comes from the speaker media kit.
