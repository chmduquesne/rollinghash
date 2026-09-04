package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

var wikiDateRE = regexp.MustCompile(`>(\d{8})/<`)

// wikimediaDumpDates lists the dump run dates currently published for a wiki,
// oldest first. Wikimedia keeps only the most recent handful.
func wikimediaDumpDates(wiki string) ([]string, error) {
	resp, err := httpGet("https://dumps.wikimedia.org/"+wiki+"/", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var dates []string
	for _, m := range wikiDateRE.FindAllStringSubmatch(string(body), -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			dates = append(dates, m[1])
		}
	}
	sort.Strings(dates)
	if len(dates) == 0 {
		return nil, fmt.Errorf("no dump dates listed for %s", wiki)
	}
	return dates, nil
}

// --- OCI registry (anonymous Docker Hub pull, no daemon) ---

type ociManifest struct {
	MediaType string `json:"mediaType"`
	Manifests []struct {
		Digest   string `json:"digest"`
		Platform struct {
			Architecture string `json:"architecture"`
			OS           string `json:"os"`
		} `json:"platform"`
	} `json:"manifests"`
	Layers []struct {
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"layers"`
}

const ociAccept = "application/vnd.oci.image.index.v1+json," +
	"application/vnd.docker.distribution.manifest.list.v2+json," +
	"application/vnd.oci.image.manifest.v1+json," +
	"application/vnd.docker.distribution.manifest.v2+json"

// ociPullFlat downloads every layer of docker.io/library/<repo>:<tag>,
// decompresses them, and concatenates them into one file at dst. Identical
// layers across tags therefore produce identical byte ranges — the dedup a
// layer-aware store would get — while a changed layer still exposes its files
// to the chunker.
func ociPullFlat(repo, tag, dst string) error {
	if fileExists(dst) {
		return nil
	}
	tokURL := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/%s:pull", repo)
	resp, err := httpGet(tokURL, nil)
	if err != nil {
		return err
	}
	var tok struct {
		Token string `json:"token"`
	}
	err = jsonBody(resp.Body, &tok)
	resp.Body.Close()
	if err != nil {
		return err
	}
	auth := map[string]string{"Authorization": "Bearer " + tok.Token, "Accept": ociAccept}

	manURL := func(ref string) string {
		return fmt.Sprintf("https://registry-1.docker.io/v2/library/%s/manifests/%s", repo, ref)
	}
	get := func(url string, v any) error {
		r, err := httpGet(url, auth)
		if err != nil {
			return err
		}
		defer r.Body.Close()
		return jsonBody(r.Body, v)
	}

	var m ociManifest
	if err := get(manURL(tag), &m); err != nil {
		return err
	}
	if len(m.Layers) == 0 && len(m.Manifests) > 0 {
		var digest string
		for _, sub := range m.Manifests {
			if sub.Platform.Architecture == "amd64" && sub.Platform.OS == "linux" {
				digest = sub.Digest
			}
		}
		if digest == "" {
			return fmt.Errorf("%s:%s: no linux/amd64 manifest", repo, tag)
		}
		if err := get(manURL(digest), &m); err != nil {
			return err
		}
	}
	if len(m.Layers) == 0 {
		return fmt.Errorf("%s:%s: no layers", repo, tag)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	for _, l := range m.Layers {
		blobURL := fmt.Sprintf("https://registry-1.docker.io/v2/library/%s/blobs/%s", repo, l.Digest)
		r, err := httpGet(blobURL, auth)
		if err != nil {
			out.Close()
			os.Remove(tmp)
			return err
		}
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			// Not gzipped (rare mediaType) — copy raw.
			if _, cerr := io.Copy(out, r.Body); cerr != nil {
				r.Body.Close()
				out.Close()
				os.Remove(tmp)
				return cerr
			}
			r.Body.Close()
			continue
		}
		_, cerr := io.Copy(out, zr)
		zr.Close()
		r.Body.Close()
		if cerr != nil {
			out.Close()
			os.Remove(tmp)
			return cerr
		}
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// --- Hugging Face ---

// hfCommits returns the commit hashes on main for a model or dataset repo,
// newest first. kind is "models" or "datasets".
func hfCommits(kind, repo string, limit int) ([]string, error) {
	url := fmt.Sprintf("https://huggingface.co/api/%s/%s/commits/main", kind, repo)
	resp, err := httpGet(url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var commits []struct {
		ID string `json:"id"`
	}
	if err := jsonBody(resp.Body, &commits); err != nil {
		return nil, err
	}
	var out []string
	for _, c := range commits {
		out = append(out, c.ID)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

// hfResolve downloads one file at a given revision. kind is "" for models or
// "datasets/" for datasets (the URL prefix HF uses).
func hfResolve(prefix, repo, rev, file, dst string) error {
	url := fmt.Sprintf("https://huggingface.co/%s%s/resolve/%s/%s", prefix, repo, rev, file)
	return download(url, dst, nil)
}
