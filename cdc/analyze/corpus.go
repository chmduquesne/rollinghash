package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// dataset is one corpus entry: an ordered set of consecutive "versions" of the
// same thing. fetch downloads/generates them under dir and returns their paths
// in order.
type dataset struct {
	name     string
	category string
	fetch    func(dir string) ([]string, error)
}

// datasets is the fixed corpus. Small entries (media, PDFs, redis) only carry
// enough bytes for the sync profile; the runner skips them for larger targets.
func datasets() []dataset {
	return []dataset{
		{
			name: "linux", category: "source",
			fetch: func(dir string) ([]string, error) {
				rels := []string{"6.6", "6.6.1", "6.6.2"}
				var out []string
				for _, r := range rels {
					tar := filepath.Join(dir, "linux-"+r+".tar")
					if !fileExists(tar) { // the .gz is deleted after gunzip
						gz := filepath.Join(dir, "linux-"+r+".tar.gz")
						url := "https://cdn.kernel.org/pub/linux/kernel/v6.x/linux-" + r + ".tar.gz"
						if err := download(url, gz, nil); err != nil {
							return nil, err
						}
						if err := gunzip(gz, tar); err != nil {
							return nil, err
						}
						_ = os.Remove(gz)
					}
					out = append(out, tar)
				}
				return out, nil
			},
		},
		{
			// Consecutive scheduled backups of a real production database:
			// Wikimedia's monthly mysqldump of the Norwegian Nynorsk Wikipedia,
			// several tables per run, gunzipped to literal `INSERT INTO …` SQL —
			// month to month mostly identical rows with a few added/changed.
			name: "wikimedia-sql", category: "db-backup",
			fetch: func(dir string) ([]string, error) {
				return wikimediaSQL(dir, "nnwiki",
					[]string{"page", "pagelinks", "categorylinks", "langlinks", "templatelinks"}, 3)
			},
		},
		{
			// A different DB-backup shape: annual snapshots of a region of the
			// live OpenStreetMap database (Geofabrik keeps them back to 2014).
			// Binary .osm.pbf (zlib per block), so an edit re-compresses a block.
			name: "osm", category: "db-backup",
			fetch: func(dir string) ([]string, error) {
				var out []string
				for _, y := range []string{"210101", "220101", "230101", "240101", "250101"} {
					dst := filepath.Join(dir, "luxembourg-"+y+".osm.pbf")
					url := "https://download.geofabrik.de/europe/luxembourg-" + y + ".osm.pbf"
					if err := download(url, dst, nil); err != nil {
						return nil, err
					}
					out = append(out, dst)
				}
				return out, nil
			},
		},
		{
			name: "hf-model", category: "ai-model",
			fetch: func(dir string) ([]string, error) {
				const repo, file = "sentence-transformers/all-MiniLM-L6-v2", "model.safetensors"
				if c, _ := filepath.Glob(filepath.Join(dir, "minilm-*.safetensors")); len(c) >= 2 {
					return sortedStrings(c), nil // already fetched
				}
				revs, err := hfCommits("models", repo, 3)
				if err != nil {
					return nil, err
				}
				var out []string
				for _, r := range revs {
					dst := filepath.Join(dir, "minilm-"+shortRev(r)+".safetensors")
					if err := hfResolve("", repo, r, file, dst); err != nil {
						return nil, err
					}
					out = append(out, dst)
				}
				return out, nil
			},
		},
		{
			name: "hf-dataset", category: "ai-dataset",
			fetch: func(dir string) ([]string, error) {
				const repo = "Salesforce/wikitext"
				files := []string{
					"wikitext-103-raw-v1/train-00000-of-00002.parquet",
					"wikitext-103-raw-v1/train-00001-of-00002.parquet",
					"wikitext-103-raw-v1/validation-00000-of-00001.parquet",
					"wikitext-103-raw-v1/test-00000-of-00001.parquet",
				}
				var out []string
				for _, f := range files {
					dst := filepath.Join(dir, filepath.Base(f))
					if err := hfResolve("datasets/", repo, "main", f, dst); err != nil {
						return nil, err
					}
					out = append(out, dst)
				}
				return out, nil
			},
		},
		{
			name: "docker", category: "docker",
			fetch: func(dir string) ([]string, error) {
				var out []string
				for _, t := range []string{"3.12.0-slim", "3.12.1-slim", "3.12.2-slim", "3.12.3-slim"} {
					dst := filepath.Join(dir, "python-"+t+".layers")
					if err := ociPullFlat("python", t, dst); err != nil {
						return nil, err
					}
					out = append(out, dst)
				}
				return out, nil
			},
		},
		{
			name: "redis", category: "source",
			fetch: func(dir string) ([]string, error) {
				var out []string
				for _, t := range []string{"7.2.0", "7.2.1", "7.2.2", "7.2.3"} {
					gz := filepath.Join(dir, "redis-"+t+".tar.gz")
					tar := filepath.Join(dir, "redis-"+t+".tar")
					if err := download("https://github.com/redis/redis/archive/refs/tags/"+t+".tar.gz", gz, nil); err != nil {
						return nil, err
					}
					if err := gunzip(gz, tar); err != nil {
						return nil, err
					}
					out = append(out, tar)
				}
				return out, nil
			},
		},
		{
			name: "arxiv", category: "pdf",
			fetch: func(dir string) ([]string, error) {
				var out []string
				for v := 1; v <= 5; v++ {
					dst := filepath.Join(dir, fmt.Sprintf("1706.03762v%d.pdf", v))
					if err := download(fmt.Sprintf("https://arxiv.org/pdf/1706.03762v%d", v), dst, nil); err != nil {
						return nil, err
					}
					out = append(out, dst)
				}
				return out, nil
			},
		},
		{
			name: "photos", category: "photos",
			fetch: func(dir string) ([]string, error) {
				var out []string
				for _, id := range []int{4, 8, 19} {
					base := filepath.Join(dir, fmt.Sprintf("kodim%02d.jpg", id))
					png := base + ".png"
					if err := download(fmt.Sprintf("https://r0k.us/graphics/kodak/kodak/kodim%02d.png", id), png, nil); err != nil {
						return nil, err
					}
					if err := pngToJPEG(png, base); err != nil {
						return nil, err
					}
					vs, err := jpegVersions(base, dir, 4)
					if err != nil {
						return nil, err
					}
					out = append(out, vs...)
				}
				return out, nil
			},
		},
		{
			name: "video", category: "video",
			fetch: func(dir string) ([]string, error) {
				base := filepath.Join(dir, "bbb.mp4")
				url := "https://test-videos.co.uk/vids/bigbuckbunny/mp4/h264/360/Big_Buck_Bunny_360_10s_5MB.mp4"
				if err := download(url, base, nil); err != nil {
					return nil, err
				}
				return videoVersions(base, dir)
			},
		},
	}
}

// wikimediaSQL fetches the last `n` dump runs of a wiki, concatenating the given
// gunzipped table dumps into one .sql file per run.
func wikimediaSQL(dir, wiki string, tables []string, n int) ([]string, error) {
	if c, _ := filepath.Glob(filepath.Join(dir, wiki+"-*.sql")); len(c) >= 2 {
		return sortedStrings(c), nil // already fetched+concatenated
	}
	dates, err := wikimediaDumpDates(wiki)
	if err != nil {
		return nil, err
	}
	if len(dates) > n {
		dates = dates[len(dates)-n:]
	}
	var out []string
	for _, d := range dates {
		sql := filepath.Join(dir, wiki+"-"+d+".sql")
		if !fileExists(sql) {
			parts := make([]string, 0, len(tables))
			ok := true
			for _, t := range tables {
				gz := filepath.Join(dir, fmt.Sprintf("%s-%s-%s.sql.gz", wiki, d, t))
				raw := gz + ".raw"
				url := fmt.Sprintf("https://dumps.wikimedia.org/%s/%s/%s-%s-%s.sql.gz", wiki, d, wiki, d, t)
				if err := download(url, gz, nil); err != nil {
					warnf("wikimedia-sql %s/%s: %v", d, t, err)
					ok = false
					break
				}
				if err := gunzip(gz, raw); err != nil {
					return nil, err
				}
				parts = append(parts, raw)
			}
			if !ok {
				continue
			}
			if err := concatFiles(parts, sql); err != nil {
				return nil, err
			}
		}
		out = append(out, sql)
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("only %d usable dump(s) — Wikimedia prunes old runs", len(out))
	}
	return out, nil
}

func shortRev(r string) string {
	if len(r) > 8 {
		return r[:8]
	}
	return r
}

// loadDataset fetches (once, cached) a dataset and returns its member paths.
func loadDataset(d dataset, cacheRoot string) ([]string, error) {
	dir := filepath.Join(cacheRoot, d.name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return d.fetch(dir)
}
