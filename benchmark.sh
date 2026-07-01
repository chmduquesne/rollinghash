#!/usr/bin/env bash
# Regenerates benchmark.md from a fresh benchstat run across every public
# interface (Roll, Chunker, ChunkWriter, BatchRoller, BatchWriter) and hash.
#
# Usage: ./benchmark.sh [count] [benchtime]
set -euo pipefail
cd "$(dirname "$0")"

count="${1:-10}"
benchtime="${2:-2s}"
pattern='BenchmarkRolling64B|BenchmarkChunker$|BenchmarkChunkWriter$|BenchmarkBatchRoller$|BenchmarkBatchWriter$'

command -v benchstat >/dev/null || {
	echo "benchstat not found; install with: go install golang.org/x/perf/cmd/benchstat@latest" >&2
	exit 1
}

raw="$(mktemp)"
trap 'rm -f "$raw"' EXIT

go test -bench="$pattern" -run=^$ -benchtime="$benchtime" -count="$count" -timeout=30m ./... >"$raw"

{
	printf '# Benchmarks\n\n'
	printf 'Throughput (MiB/s) of every public interface, across every hash.\n'
	printf '`benchstat` summary over %s runs. Regenerate with `./benchmark.sh [count] [benchtime]`.\n\n' "$count"
	printf '```\ngo test -bench='"'"'%s'"'"' -run=^$ -benchtime=%s -count=%s ./... | benchstat -format csv -\n```\n\n' \
		"$pattern" "$benchtime" "$count"
	printf '| Benchmark | Throughput |\n'
	printf '|---|---|\n'
	# For benchmarks swept across multiple buffer sizes (BatchRoller/BatchWriter's
	# /buf=... variants), keep only the best-performing buffer size per hash.
	benchstat -format csv "$raw" 2>/dev/null | awk -F, '
		/^pkg: / { pkg=$0; sub(/^pkg: /, "", pkg); n=split(pkg, parts, "/"); hash=parts[n]; next }
		/,B\/s,CI$/ { insection=1; next }
		/^$/ { insection=0; next }
		insection && $1 != "" && $1 != "geomean" {
			name = $1
			sub(/-[0-9]+$/, "", name)
			if (name !~ /\//) name = hash "/" name
			mib = $2 / 1048576
			ci = $3
			key = name
			sub(/\/buf=[^\/]+$/, "", key)
			if (!(key in best) || mib > best[key]) {
				best[key] = mib
				bestci[key] = ci
			}
		}
		END {
			for (k in best) printf "| %s | %.1f MiB/s ± %s |\n", k, best[k], bestci[k]
		}
	' | sort
} >benchmark.md

echo "wrote benchmark.md" >&2
