package main

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Downloader fetches URLs and feeds audio files to the Organizer.
type Downloader struct {
	cfg Config
	q   *Queue
	org *Organizer
}

func newDownloader(cfg Config, q *Queue, org *Organizer) *Downloader {
	return &Downloader{cfg: cfg, q: q, org: org}
}

// run is the main worker loop. It processes pending items one at a time.
func (d *Downloader) run() {
	for {
		item := d.q.nextPending()
		if item == nil {
			// Wait for notification of new items.
			<-d.q.notify
			continue
		}
		log.Printf("downloader: processing %s (%s)", item.ID, item.URL)
		if err := d.process(item); err != nil {
			log.Printf("downloader: error processing %s: %v", item.ID, err)
			d.q.update(item.ID, StatusError, withError(err.Error()))
		}
		// Check if more pending items exist immediately.
		if d.q.hasPending() {
			select {
			case d.q.notify <- struct{}{}:
			default:
			}
		}
	}
}

// process downloads the URL and organizes the resulting audio files.
func (d *Downloader) process(item *QueueItem) error {
	// Download the file.
	d.q.update(item.ID, StatusDownloading)
	destPath, err := d.download(item)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer os.Remove(destPath)

	d.q.update(item.ID, StatusExtracting, withFileName(filepath.Base(destPath)))

	// Collect audio file paths (extract archive if needed).
	audioPaths, tmpDir, err := d.extract(destPath)
	if tmpDir != "" {
		defer os.RemoveAll(tmpDir)
	}
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	if len(audioPaths) == 0 {
		return fmt.Errorf("no audio files found in download")
	}

	// Organize each audio file into the library.
	d.q.update(item.ID, StatusOrganizing)
	var organized []string
	for _, ap := range audioPaths {
		dest, err := d.org.organize(ap, item.Library)
		if err != nil {
			log.Printf("downloader: organize %s: %v", ap, err)
			continue
		}
		organized = append(organized, dest)
	}
	if len(organized) == 0 {
		return fmt.Errorf("failed to organize any audio files")
	}

	d.q.update(item.ID, StatusDone, withFiles(organized))
	return nil
}

// download fetches the URL to a temporary file and returns its path.
func (d *Downloader) download(item *QueueItem) (string, error) {
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(item.URL) //nolint:noctx
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned %s", resp.Status)
	}

	// Derive a safe temp file name.
	ext := extensionFromResponse(resp, item.URL)
	f, err := os.CreateTemp(d.cfg.DownloadDir, "dostobot-*"+ext)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// extensionFromResponse guesses a file extension from the Content-Disposition
// header or the URL path.
func extensionFromResponse(resp *http.Response, rawURL string) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		// filename="foo.zip"
		for _, part := range strings.Split(cd, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "filename=") {
				name := strings.Trim(strings.TrimPrefix(part, "filename="), `"'`)
				if ext := filepath.Ext(name); ext != "" {
					return ext
				}
			}
		}
	}
	if ext := filepath.Ext(rawURL); ext != "" && len(ext) <= 5 {
		return ext
	}
	return ""
}

// audioExtensions is the set of recognized audio file extensions.
var audioExtensions = map[string]bool{
	".flac": true,
	".mp3":  true,
	".m4a":  true,
	".aac":  true,
	".ogg":  true,
	".opus": true,
	".wav":  true,
	".aiff": true,
	".ape":  true,
	".wv":   true,
}

func isAudio(name string) bool {
	return audioExtensions[strings.ToLower(filepath.Ext(name))]
}

// extract unpacks an archive (zip/tar.gz/tar.bz2) or returns the path
// unchanged if it is already an audio file.  tmpDir is set when a temporary
// extraction directory was created and the caller should clean it up.
func (d *Downloader) extract(path string) (audioPaths []string, tmpDir string, err error) {
	lower := strings.ToLower(path)

	switch {
	case strings.HasSuffix(lower, ".zip"):
		return d.extractZip(path)
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return d.extractTar(path, "gz")
	case strings.HasSuffix(lower, ".tar.bz2") || strings.HasSuffix(lower, ".tbz2"):
		return d.extractTar(path, "bz2")
	case strings.HasSuffix(lower, ".tar"):
		return d.extractTar(path, "")
	default:
		if isAudio(path) {
			return []string{path}, "", nil
		}
		// Try zip anyway (some stores serve zip without extension)
		if paths, td, e := d.extractZip(path); e == nil && len(paths) > 0 {
			return paths, td, nil
		}
		return nil, "", fmt.Errorf("unsupported file format: %s", filepath.Base(path))
	}
}

func (d *Downloader) extractZip(path string) ([]string, string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, "", err
	}
	defer r.Close()

	tmpDir, err := os.MkdirTemp(d.cfg.DownloadDir, "dostobot-extract-*")
	if err != nil {
		return nil, "", err
	}

	var audioPaths []string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Guard against path traversal
		cleanName := filepath.Clean(f.Name)
		if strings.HasPrefix(cleanName, "..") {
			continue
		}
		destPath := filepath.Join(tmpDir, cleanName)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return nil, tmpDir, err
		}
		if err := extractZipFile(f, destPath); err != nil {
			return nil, tmpDir, err
		}
		if isAudio(destPath) {
			audioPaths = append(audioPaths, destPath)
		}
	}
	return audioPaths, tmpDir, nil
}

func extractZipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc) //nolint:gosec
	return err
}

func (d *Downloader) extractTar(path, compression string) ([]string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()

	var reader io.Reader = f
	switch compression {
	case "gz":
		gr, err := gzip.NewReader(f)
		if err != nil {
			return nil, "", err
		}
		defer gr.Close()
		reader = gr
	case "bz2":
		reader = bzip2.NewReader(f)
	}

	tmpDir, err := os.MkdirTemp(d.cfg.DownloadDir, "dostobot-extract-*")
	if err != nil {
		return nil, "", err
	}

	tr := tar.NewReader(reader)
	var audioPaths []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, tmpDir, err
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		// Guard against path traversal
		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") {
			continue
		}
		destPath := filepath.Join(tmpDir, cleanName)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return nil, tmpDir, err
		}
		out, err := os.Create(destPath)
		if err != nil {
			return nil, tmpDir, err
		}
		_, copyErr := io.Copy(out, tr) //nolint:gosec
		out.Close()
		if copyErr != nil {
			return nil, tmpDir, copyErr
		}
		if isAudio(destPath) {
			audioPaths = append(audioPaths, destPath)
		}
	}
	return audioPaths, tmpDir, nil
}
