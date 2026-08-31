#!/usr/bin/env bash
# Regenerates benchmark.md from a fresh benchstat run across every public
# interface (Roll, Chunker, ChunkWriter, BatchRoller, BatchWriter) and hash.
#
# Usage: ./benchmark.sh [--force-boost] [count] [benchtime]
set -euo pipefail
cd "$(dirname "$0")"

force_boost=false
args=()
for arg in "$@"; do
	if [ "$arg" = "--force-boost" ]; then
		force_boost=true
	else
		args+=("$arg")
	fi
done
set -- "${args[@]+"${args[@]}"}"

count="${1:-10}"
benchtime="${2:-2s}"
pattern='BenchmarkRolling64B|BenchmarkChunker$|BenchmarkChunkWriter$|BenchmarkBatchRoller$|BenchmarkBatchWriter$'

# The script refuses to run with boost detected as enabled; pass
# --force-boost to override (e.g. on a machine where boost is
# intentionally left on, or where you accept the bias/heat for a quick,
# rough run). Without boost disabled, whichever hash happens to be
# benchmarked first runs on a cool, boosted CPU while later ones run
# throttled, biasing results by benchmark order
amd_boost=/sys/devices/system/cpu/cpufreq/boost
intel_no_turbo=/sys/devices/system/cpu/intel_pstate/no_turbo
reenable_cmd=""
if ! $force_boost; then
	if [ -f "$amd_boost" ] && [ "$(cat "$amd_boost")" != "0" ]; then
		echo "CPU boost is enabled ($amd_boost = $(cat "$amd_boost")). A run this long will thermal-throttle partway through, biasing whichever hash runs first, and can run the CPU dangerously hot. Disable it first:" >&2
		echo "  echo 0 | sudo tee $amd_boost" >&2
		echo "or pass --force-boost to run anyway." >&2
		exit 1
	elif [ -f "$intel_no_turbo" ] && [ "$(cat "$intel_no_turbo")" != "1" ]; then
		echo "CPU turbo is enabled ($intel_no_turbo = $(cat "$intel_no_turbo")). A run this long will thermal-throttle partway through, biasing whichever hash runs first, and can run the CPU dangerously hot. Disable it first:" >&2
		echo "  echo 1 | sudo tee $intel_no_turbo" >&2
		echo "or pass --force-boost to run anyway." >&2
		exit 1
	fi
	# Neither control file exists (unknown driver/platform): can't verify
	# boost state, so proceed without blocking.
	if [ -f "$amd_boost" ]; then
		reenable_cmd="echo 1 | sudo tee $amd_boost"
	elif [ -f "$intel_no_turbo" ]; then
		reenable_cmd="echo 0 | sudo tee $intel_no_turbo"
	fi
fi

# We disabled boost ourselves (or the caller did, before running us) to get
# a fair, low-variance run; this script has no way to restore it reliably
# (sudo here would need a password prompt to survive a 20-40 minute run,
# possibly with no TTY attached if run in the background), so just remind
# the caller to do it themselves once we're done, success or not.
raw=""
cleanup() {
	if [ -n "$raw" ]; then
		rm -f "$raw"
	fi
	if [ -n "$reenable_cmd" ]; then
		echo "Re-enable CPU boost now that the benchmark is done: $reenable_cmd" >&2
	fi
	return 0
}
trap cleanup EXIT

command -v benchstat >/dev/null || {
	echo "benchstat not found; install with: go install golang.org/x/perf/cmd/benchstat@latest" >&2
	exit 1
}

raw="$(mktemp)"

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
