package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

var offline bool

const userAgent = "rollinghash-cdc-analyze/1 (+https://github.com/chmduquesne/rollinghash)"

// httpGet issues a GET with the shared User-Agent and returns the body, which
// the caller must close. Non-2xx is an error.
func httpGet(url string, hdr map[string]string) (*http.Response, error) {
	if offline {
		return nil, fmt.Errorf("offline: refusing to fetch %s", url)
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp, nil
}

// download fetches url into dst unless dst already exists.
func download(url, dst string, hdr map[string]string) error {
	if fileExists(dst) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	resp, err := httpGet(url, hdr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// gunzip writes the decompressed contents of src (a .gz file) to dst.
func gunzip(src, dst string) error {
	if fileExists(dst) {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	zr, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer zr.Close()

	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, zr); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// concatFiles writes the byte concatenation of srcs to dst.
func concatFiles(srcs []string, dst string) error {
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	for _, s := range srcs {
		in, err := os.Open(s)
		if err != nil {
			out.Close()
			os.Remove(tmp)
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		if err != nil {
			out.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func jsonBody(r io.Reader, v any) error { return json.NewDecoder(r).Decode(v) }

func sortedStrings(s []string) []string {
	c := append([]string(nil), s...)
	sort.Strings(c)
	return c
}
