package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
)

// pngToJPEG decodes a PNG and writes it as a quality-92 JPEG at dst.
func pngToJPEG(src, dst string) error {
	if fileExists(dst) {
		return nil
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("decode %s: %w", src, err)
	}
	w, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer w.Close()
	return jpeg.Encode(w, img, &jpeg.Options{Quality: 92})
}

// jpegVersions creates an edited-and-re-saved chain from base: v0 is a plain
// re-encode, then each further version nudges the brightness of a small
// rectangle and re-encodes at a slightly lower quality. This is the "I tweaked
// a photo and saved again" case — a tiny visual change, but the lossy codec
// re-emits its whole entropy stream, so CDC rarely dedups between versions.
func jpegVersions(base, dir string, n int) ([]string, error) {
	f, err := os.Open(base)
	if err != nil {
		return nil, err
	}
	src, err := jpeg.Decode(f)
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", base, err)
	}

	rgba := image.NewRGBA(src.Bounds())
	draw.Draw(rgba, rgba.Bounds(), src, src.Bounds().Min, draw.Src)
	b := rgba.Bounds()

	name := trimExt(filepath.Base(base))
	out := []string{base}
	for v := range n {
		if v > 0 {
			x0 := b.Min.X + (v*37)%max(1, b.Dx()-32)
			y0 := b.Min.Y + (v*53)%max(1, b.Dy()-32)
			for y := y0; y < y0+32 && y < b.Max.Y; y++ {
				for x := x0; x < x0+32 && x < b.Max.X; x++ {
					c := rgba.RGBAAt(x, y)
					c.R = clamp8(int(c.R) + 12)
					c.G = clamp8(int(c.G) + 12)
					c.B = clamp8(int(c.B) + 12)
					rgba.SetRGBA(x, y, c)
				}
			}
		}
		p := filepath.Join(dir, fmt.Sprintf("%s.v%d.jpg", name, v))
		if !fileExists(p) {
			w, err := os.Create(p)
			if err != nil {
				return nil, err
			}
			err = jpeg.Encode(w, rgba, &jpeg.Options{Quality: 90 - v})
			w.Close()
			if err != nil {
				return nil, err
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// videoVersions builds an edited chain from base using ffmpeg: a re-encode, a
// tail trim, and a brightness nudge. Returns just [base] (and no error) when
// ffmpeg is not installed.
func videoVersions(base, dir string) ([]string, error) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		warnf("ffmpeg not found: using %s as a single version", filepath.Base(base))
		return []string{base}, nil
	}
	name := trimExt(filepath.Base(base))
	steps := []struct {
		suffix string
		args   []string
	}{
		{"reencode", []string{"-c:v", "libx264", "-crf", "28", "-preset", "veryfast"}},
		{"trim", []string{"-t", "5", "-c:v", "libx264", "-crf", "28", "-preset", "veryfast"}},
		{"bright", []string{"-vf", "eq=brightness=0.06", "-c:v", "libx264", "-crf", "28", "-preset", "veryfast"}},
	}
	out := []string{base}
	for _, s := range steps {
		p := filepath.Join(dir, fmt.Sprintf("%s.%s.mp4", name, s.suffix))
		if !fileExists(p) {
			args := append([]string{"-y", "-loglevel", "error", "-i", base}, s.args...)
			args = append(args, "-an", p)
			if err := exec.Command(ff, args...).Run(); err != nil {
				return nil, fmt.Errorf("ffmpeg %s: %w", s.suffix, err)
			}
		}
		out = append(out, p)
	}
	return out, nil
}

func clamp8(v int) uint8 { return uint8(min(255, max(0, v))) }

func trimExt(s string) string { return s[:len(s)-len(filepath.Ext(s))] }
