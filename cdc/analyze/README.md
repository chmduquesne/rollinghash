# cdc/analyze

A one-command comparison of every content-defined chunker in this repo over a
fixed multi-format, real-world corpus, at the chunk sizes production systems
actually use.

It is a **nested module** (own `go.mod`) so this tool's own dependencies never
leak into the dependency-free root, exactly like `cdc/compat/*/bench`.

```sh
cd cdc/analyze
go run .                              # every profile, whole corpus (~20–45 min, ~1.7 GB fetch)
go run . -profile backup              # just the 1 MiB profile
go run . -profile sync -only redis    # one profile, one dataset
go run . -avg 393216                  # a custom target (min=avg/32, max=avg*8)
go run . -avg 393216 -min 65536       # …with an explicit min/max override
go run . -timeout 3m                  # widen the per-cell time budget
go run . -json > results.json
go run . -offline -only osm           # reuse the cache, no network
```

## Size profiles

Every profile is one **target** chunk size; `min = target/32` and
`max = target*8` are always derived from it, so the only thing that changes
between profiles is the target — every algorithm sees the same size envelope,
scaled.

| profile | target | ⇒ min | ⇒ max | models the defaults of |
|---|--:|--:|--:|---|
| `sync` | 64 KiB | 2 KiB | 512 KiB | casync / desync (Lennart Poettering) |
| `artifact` | 256 KiB | 8 KiB | 2 MiB | Google Stadia `cdc_stream`, Bazel RE-API / buildbarn |
| `backup` | 1 MiB | 32 KiB | 8 MiB | restic, plakar (borg 2 MiB, kopia 4 MiB) |

8 KiB (the LBFS-era default some libraries still ship) is not one of these — no
production system uses it; hence these three instead.

## What it measures

Methodology ported from `go-cdc-chunkers`' `cmd/cdc` (`stats.go` + `resync.go`):

- **saved%** — `1 − uniqueBytes/totalBytes`. Each dataset is an ordered set of
  *consecutive versions* of one thing, streamed in sequence against a single
  SHA-256 set, so cross-version duplication drives the number. Higher is better —
  **but read it against the `avg` column**: a chunker below the target dedups
  more for free.
- **size distribution** — `avg p95 max stddev`; `size-cv` (stddev/avg) in the
  roll-up — lower is a tighter, more predictable distribution.
- **resync%** — chunk the dataset's largest file (capped at 128 MiB), apply
  `-edits` single-byte insertions (which shift the tail), re-chunk, and measure
  the fraction of the edited stream still carried by chunks the original had.
  This is the metric that actually separates good CDC from bad. Walked in order
  (upstream sums over the dedup'd set, understating self-similar inputs).
- **MB/s** — chunking only.

A cell that exceeds `-timeout` (default 90 s) shows `timeout` — expect this for
`repmaxsfxcdc` at the `backup` target (its full-horizon suffix rescan is
`O(min·horizon)` per chunk).

`ultracdc` does not scale past ~64 KiB chunks regardless of the target you ask
for; `maxpcdc`'s size is data-dependent and often far off target — the `avg`
column shows the reality in both cases.

## Corpus

Fixed list, cached under `$XDG_CACHE_HOME/rollinghash-cdc-corpus` (override with
`-cache` / `$ROLLINGHASH_CORPUS`). Files are streamed from disk, never fully
loaded. A failed fetch logs a warning and skips that dataset; a dataset whose
largest member would give fewer than 30 chunks at a profile's target is skipped
for that profile.

| dataset | category | source | profiles |
|---|---|---|---|
| `linux` | source | kernel.org linux-6.6, 6.6.1, 6.6.2 (`.tar` streams) | all |
| `wikimedia-sql` | db-backup | Wikimedia monthly `mysqldump` of `nnwiki` (Norwegian Nynorsk) — `page`+`pagelinks`+`categorylinks`+`langlinks`+`templatelinks`, gunzipped+concatenated, last 3 runs from the live index | all |
| `osm` | db-backup | Geofabrik annual `.osm.pbf` of `europe/luxembourg` from the live OpenStreetMap DB, 5 years | all |
| `hf-model` | ai-model | `sentence-transformers/all-MiniLM-L6-v2` `model.safetensors` at its last 3 commits | sync + artifact |
| `hf-dataset` | ai-dataset | `Salesforce/wikitext` `wikitext-103-raw-v1` parquet shards | sync + artifact |
| `docker` | docker | `python:3.12.{0..3}-slim`, layers pulled from the registry (no daemon) and flattened | sync (+ artifact) |
| `redis` | source | GitHub release tarballs 7.2.0–7.2.3 (`.tar`) | sync + artifact |
| `arxiv` | pdf | "Attention Is All You Need", arXiv revisions v1–v5 | sync |
| `photos` | photos | Kodak images → JPEG, then a generated edit-and-re-save chain (`image/jpeg`) | sync |
| `video` | video | one Big Buck Bunny clip + an `ffmpeg` edit chain (clip alone if no `ffmpeg`) | sync |

`wikimedia-sql` is the real "consecutive scheduled backups of a production
database" case — literal `INSERT INTO …` text, mostly unchanged month to month.
Wikimedia prunes old runs, so the fetcher reads the current dump index and takes
what exists, skipping the dataset if fewer than two remain. `osm` is a binary
DB-backup shape (zlib per block). `photos`/`video` are the "edited and re-saved a
compressed file" case — a one-pixel change re-emits the whole entropy stream, so
CDC dedup between versions is near zero (a real result worth seeing).

## Adding a dataset

Append to `datasets()` in `corpus.go`: `name`, `category`, and a
`fetch(dir) ([]string, error)` returning the ordered member paths (cache by
checking `fileExists`). Helpers: `download`, `gunzip`, `concatFiles`,
`ociPullFlat`, `hfCommits`/`hfResolve`, `wikimediaDumpDates`, `jpegVersions`,
`videoVersions`.

## Not covered yet

Database-backup datasets that need a local generator (seed → mutate → dump) and
Office-document datasets (template → edit ×N). No plots — `-json` (nested
profile → dataset → rows) is the hook for an external plotter.
