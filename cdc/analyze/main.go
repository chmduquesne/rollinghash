// Command analyze downloads a fixed multi-format corpus (consecutive versions of
// source trees, database backups, docker images, PDFs, photos, video, AI
// models/datasets) and runs every CDC algorithm in this repo — plus the real
// go-cdc-chunkers ones — over it at three real-world size targets, printing
// chunk-size distribution, deduplication ratio and boundary-resync resistance.
//
//	cd cdc/analyze && go run .                 # all profiles, whole corpus
//	go run . -profile backup -only wikimedia-sql
//	go run . -avg 262144                       # a custom target
//	go run . -json > results.json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	verbose     bool
	parallelRun bool
)

func warnf(format string, a ...any) { fmt.Fprintf(os.Stderr, "warning: "+format+"\n", a...) }

func logf(format string, a ...any) {
	if verbose {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
	}
}

func defaultCache() string {
	if d := os.Getenv("ROLLINGHASH_CORPUS"); d != "" {
		return d
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "rollinghash-cdc-corpus")
	}
	return filepath.Join(os.TempDir(), "rollinghash-cdc-corpus")
}

// chunksTooFew skips a (dataset, profile) pair whose largest member would not
// produce enough chunks at that target for the numbers to mean anything.
const minChunksForMeaning = 30

type profileResult struct {
	Profile  string          `json:"profile"`
	Target   int             `json:"target_bytes"`
	Min      int             `json:"min_bytes"`
	Max      int             `json:"max_bytes"`
	Datasets []datasetResult `json:"datasets"`
}

type datasetResult struct {
	Dataset  string    `json:"dataset"`
	Category string    `json:"category"`
	Files    int       `json:"files"`
	Bytes    int64     `json:"bytes"`
	Rows     []algoRow `json:"rows"`
}

type algoRow struct {
	Algorithm string  `json:"algorithm"`
	Timeout   bool    `json:"timeout,omitempty"`
	Chunks    int     `json:"chunks,omitempty"`
	P50       int     `json:"p50,omitempty"`
	Avg       int     `json:"avg,omitempty"`
	P95       int     `json:"p95,omitempty"`
	Max       int     `json:"max,omitempty"`
	Stddev    float64 `json:"stddev,omitempty"`
	DedupPct  float64 `json:"dedup_saved_pct,omitempty"`
	ResyncPct float64 `json:"resync_shared_pct,omitempty"`
	MBs       float64 `json:"throughput_mbs,omitempty"`
}

func main() {
	var (
		profileArg     string
		only           string
		cache          string
		edits          int
		timeout        time.Duration
		jobs           int
		asJSON         bool
		avg, cmin, cmx int
	)
	flag.StringVar(&profileArg, "profile", "sync,artifact,backup", "comma-separated profiles to run")
	flag.StringVar(&only, "only", "", "comma-separated categories or dataset names (default: all)")
	flag.StringVar(&cache, "cache", defaultCache(), "corpus cache directory")
	flag.BoolVar(&offline, "offline", false, "use only already-cached files; never hit the network")
	flag.IntVar(&edits, "edits", 16, "single-byte insertions for the resync metric")
	flag.DurationVar(&timeout, "timeout", 90*time.Second, "per (algorithm,dataset,profile) time budget")
	flag.IntVar(&jobs, "jobs", runtime.NumCPU(), "algorithms to chunk in parallel (1 = serial, needed for trustworthy MB/s)")
	flag.IntVar(&avg, "avg", 0, "custom target chunk size (bytes); adds a 'custom' profile")
	flag.IntVar(&cmin, "min", 0, "custom min chunk size (bytes); default target/32")
	flag.IntVar(&cmx, "max", 0, "custom max chunk size (bytes); default target*8")
	flag.BoolVar(&asJSON, "json", false, "emit results as JSON")
	flag.BoolVar(&verbose, "v", false, "verbose progress")
	flag.Parse()

	parallelRun = jobs > 1
	profiles := selectProfiles(profileArg, avg, cmin, cmx)
	if len(profiles) == 0 {
		fmt.Fprintln(os.Stderr, "no profiles selected")
		os.Exit(2)
	}

	filter := map[string]bool{}
	for s := range strings.SplitSeq(only, ",") {
		if s = strings.TrimSpace(s); s != "" {
			filter[s] = true
		}
	}
	keep := func(d dataset) bool {
		return len(filter) == 0 || filter[d.name] || filter[d.category]
	}

	// Fetch each dataset once; every profile streams the same files from disk.
	type loaded struct {
		d           dataset
		paths       []string
		total       int64
		biggest     string
		biggestSize int64
	}
	var corpus []loaded
	for _, d := range datasets() {
		if !keep(d) {
			continue
		}
		logf("[%s] fetching…", d.name)
		paths, err := loadDataset(d, cache)
		if err != nil {
			warnf("%s: skipped (%v)", d.name, err)
			continue
		}
		l := loaded{d: d, paths: paths}
		for _, p := range paths {
			st, err := os.Stat(p)
			if err != nil {
				warnf("%s: %v", d.name, err)
				continue
			}
			l.total += st.Size()
			if st.Size() > l.biggestSize {
				l.biggest, l.biggestSize = p, st.Size()
			}
		}
		corpus = append(corpus, l)
		logf("[%s] %d file(s), %s", d.name, len(paths), humanBytes(l.total))
	}

	var results []profileResult
	for _, p := range profiles {
		sz := p.sizes()
		algos := registry(sz)
		pr := profileResult{Profile: p.name, Target: p.target, Min: sz.min, Max: sz.max}
		for _, l := range corpus {
			if int(l.biggestSize)/p.target < minChunksForMeaning {
				logf("[%s/%s] skipped: largest member only ~%d chunks at this target",
					p.name, l.d.name, int(l.biggestSize)/max(p.target, 1))
				continue
			}
			dr := datasetResult{Dataset: l.d.name, Category: l.d.category, Files: len(l.paths), Bytes: l.total}
			dr.Rows = runDataset(p.name, l.d.name, algos, l.paths, l.biggest, edits, timeout, jobs)
			pr.Datasets = append(pr.Datasets, dr)
		}
		results = append(results, pr)
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
		return
	}
	render(results, edits)
}

// runDataset measures every algorithm on one dataset, up to `jobs` at once.
// Rows come back in registry order regardless of completion order.
func runDataset(prof, ds string, algos []namedChunker, paths []string, biggest string, edits int, budget time.Duration, jobs int) []algoRow {
	if jobs < 1 {
		jobs = 1
	}
	rows := make([]algoRow, len(algos))
	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	for i, a := range algos {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, a namedChunker) {
			defer wg.Done()
			defer func() { <-sem }()
			logf("[%s/%s] %s…", prof, ds, a.name)
			rows[i] = runCell(a, paths, biggest, edits, budget)
		}(i, a)
	}
	wg.Wait()
	return rows
}

// runCell measures one algorithm on one dataset under the time budget.
func runCell(a namedChunker, paths []string, biggest string, edits int, budget time.Duration) algoRow {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	res, err := measure(ctx, a.name, a.fn, paths)
	if err != nil {
		return algoRow{Algorithm: a.name, Timeout: true}
	}
	if sh, err := resync(ctx, a.fn, biggest, edits, 1); err == nil {
		res.resyncShared = sh
	}
	di := res.distribution()
	return algoRow{
		Algorithm: a.name, Chunks: res.chunks,
		P50: di.p50, Avg: di.avg, P95: di.p95, Max: di.max, Stddev: di.stddev,
		DedupPct: res.savedPct(), ResyncPct: 100 * res.resyncShared, MBs: res.throughputMBs(),
	}
}

func selectProfiles(arg string, avg, cmin, cmx int) []profile {
	std := map[string]profile{}
	for _, p := range standardProfiles() {
		std[p.name] = p
	}
	var out []profile
	for s := range strings.SplitSeq(arg, ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		if p, ok := std[s]; ok {
			out = append(out, p)
		} else {
			warnf("unknown profile %q (have sync, artifact, backup)", s)
		}
	}
	if avg > 0 {
		p := profile{name: "custom", target: avg}
		if cmin > 0 || cmx > 0 {
			s := sizesFor(avg)
			if cmin > 0 {
				s.min = cmin
			}
			if cmx > 0 {
				s.max = cmx
			}
			p.override = &s
		}
		out = append(out, p)
	}
	return out
}

// --- rendering ---

func render(results []profileResult, edits int) {
	for _, pr := range results {
		fmt.Printf("\n########## profile %s — target %s (min %s, max %s) ##########\n",
			pr.Profile, humanBytes(int64(pr.Target)),
			humanBytes(int64(pr.Min)), humanBytes(int64(pr.Max)))
		for _, d := range pr.Datasets {
			fmt.Printf("\n%s  [%s]  %d file(s), %s\n", d.Dataset, d.Category, d.Files, humanBytes(d.Bytes))
			fmt.Printf("  %-22s %8s %8s %8s %9s %8s %8s %9s\n",
				"algorithm", "chunks", "avg", "p95", "max", "stddev", "saved%", "resync%")
			rows := slices.Clone(d.Rows)
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].Timeout != rows[j].Timeout {
					return !rows[i].Timeout
				}
				return rows[i].DedupPct > rows[j].DedupPct
			})
			for _, r := range rows {
				if r.Timeout {
					fmt.Printf("  %-22s %8s\n", r.Algorithm, "timeout")
					continue
				}
				fmt.Printf("  %-22s %8d %8d %8d %9d %8.0f %8.2f %9.2f\n",
					r.Algorithm, r.Chunks, r.Avg, r.P95, r.Max, r.Stddev, r.DedupPct, r.ResyncPct)
			}
		}
		printRollup(pr)
	}
	printCrossProfile(results)
	fmt.Printf("\nprofiles model real systems: sync 64K ≈ casync/desync, artifact 256K ≈ Stadia/\n")
	fmt.Printf("buildbarn, backup 1M ≈ restic/plakar (min=target/32, max=target*8).\n")
	fmt.Printf("saved%% = 1 - unique/total.  resync%% = edited bytes still shared after %d insertions.\n", edits)
	fmt.Printf("Read saved%% against avg — a chunker below target dedups more for free.\n")
	if parallelRun {
		fmt.Printf("MB/s is wall-clock and contended (-jobs > 1); use -jobs 1 for throughput.\n")
	}
}

type aggRow struct {
	algo                            string
	n                               int
	dedup, resync, mbs, stddevRatio float64
}

func aggregate(pr profileResult) []*aggRow {
	m := map[string]*aggRow{}
	var order []string
	for _, d := range pr.Datasets {
		for _, r := range d.Rows {
			if r.Timeout {
				continue
			}
			a := m[r.Algorithm]
			if a == nil {
				a = &aggRow{algo: r.Algorithm}
				m[r.Algorithm] = a
				order = append(order, r.Algorithm)
			}
			a.n++
			a.dedup += r.DedupPct
			a.resync += r.ResyncPct
			a.mbs += r.MBs
			a.stddevRatio += r.Stddev / float64(max(r.Avg, 1))
		}
	}
	out := make([]*aggRow, 0, len(order))
	for _, k := range order {
		out = append(out, m[k])
	}
	return out
}

func (a *aggRow) means() (dedup, resync, mbs, cv float64) {
	n := float64(max(a.n, 1))
	return a.dedup / n, a.resync / n, a.mbs / n, a.stddevRatio / n
}

func printRollup(pr profileResult) {
	rows := aggregate(pr)
	if len(rows) == 0 {
		return
	}
	fmt.Printf("\n--- %s roll-up (mean across %d dataset(s)) ---\n", pr.Profile, len(pr.Datasets))
	fmt.Printf("  %-22s %10s %10s %10s %10s\n", "algorithm", "saved%", "resync%", "size-cv", "MB/s")
	byDedup := slices.Clone(rows)
	sort.SliceStable(byDedup, func(i, j int) bool {
		di, _, _, _ := byDedup[i].means()
		dj, _, _, _ := byDedup[j].means()
		return di > dj
	})
	for _, r := range byDedup {
		d, rs, m, cv := r.means()
		fmt.Printf("  %-22s %10.2f %10.2f %10.2f %10.0f\n", r.algo, d, rs, cv, m)
	}
}

func printCrossProfile(results []profileResult) {
	if len(results) < 2 {
		return
	}
	fmt.Printf("\n=== across profiles: %s ===\n", strings.Join(profileNames(results), " → "))
	type cell struct {
		d, r float64
		ok   bool
	}
	perAlgo := map[string][]cell{}
	var order []string
	for pi, pr := range results {
		for _, a := range aggregate(pr) {
			if _, seen := perAlgo[a.algo]; !seen {
				order = append(order, a.algo)
				perAlgo[a.algo] = make([]cell, len(results))
			}
			d, rs, _, _ := a.means()
			perAlgo[a.algo][pi] = cell{d, rs, true}
		}
	}
	for _, algo := range order {
		ds := make([]string, len(results))
		rss := make([]string, len(results))
		for i, c := range perAlgo[algo] {
			if !c.ok {
				ds[i], rss[i] = "–", "–"
				continue
			}
			ds[i] = fmt.Sprintf("%.1f", c.d)
			rss[i] = fmt.Sprintf("%.0f", c.r)
		}
		fmt.Printf("  %-22s  saved %s   resync %s\n", algo, strings.Join(ds, "→"), strings.Join(rss, "→"))
	}
}

func profileNames(results []profileResult) []string {
	n := make([]string, len(results))
	for i, r := range results {
		n[i] = r.Profile
	}
	return n
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
