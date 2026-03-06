package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/dhowden/tag"
)

// Organizer moves audio files into a structured library directory.
// Directory layout:  {LibraryDir}/{AlbumArtist}/{Album}/{NN. Title.ext}
// With disc numbers:  {LibraryDir}/{AlbumArtist}/{Album}/{DD-NN. Title.ext}
type Organizer struct {
	libraryDir string
}

func newOrganizer(libraryDir string) *Organizer {
	return &Organizer{libraryDir: libraryDir}
}

// organize reads metadata from src and moves the file into the library.
// It returns the final destination path.
func (o *Organizer) organize(src string) (string, error) {
	meta, err := readMeta(src)
	if err != nil {
		log.Printf("organizer: metadata read error for %s: %v – using filename fallback", src, err)
	}

	dest := o.destinationPath(src, meta)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return "", fmt.Errorf("create dirs: %w", err)
	}
	// If destination already exists, add a suffix rather than overwriting.
	dest = uniquePath(dest)

	if err := moveFile(src, dest); err != nil {
		return "", fmt.Errorf("move file: %w", err)
	}
	log.Printf("organizer: %s -> %s", src, dest)
	return dest, nil
}

// audioMeta holds the fields we care about for library organisation.
type audioMeta struct {
	AlbumArtist string
	Artist      string
	Album       string
	Title       string
	Track       int
	TrackTotal  int
	Disc        int
	DiscTotal   int
}

// readMeta opens the file and extracts audio metadata.
func readMeta(path string) (*audioMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, err
	}

	track, trackTotal := m.Track()
	disc, discTotal := m.Disc()
	return &audioMeta{
		AlbumArtist: strings.TrimSpace(m.AlbumArtist()),
		Artist:      strings.TrimSpace(m.Artist()),
		Album:       strings.TrimSpace(m.Album()),
		Title:       strings.TrimSpace(m.Title()),
		Track:       track,
		TrackTotal:  trackTotal,
		Disc:        disc,
		DiscTotal:   discTotal,
	}, nil
}

// destinationPath builds the target path for the file.
func (o *Organizer) destinationPath(src string, m *audioMeta) string {
	ext := strings.ToLower(filepath.Ext(src))
	baseName := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))

	// Determine the three path components with sane fallbacks.
	artist := "Unknown Artist"
	album := "Unknown Album"
	title := baseName

	if m != nil {
		if m.AlbumArtist != "" {
			artist = m.AlbumArtist
		} else if m.Artist != "" {
			artist = m.Artist
		}
		if m.Album != "" {
			album = m.Album
		}
		if m.Title != "" {
			title = m.Title
		}
	}

	prefix := trackPrefix(m)
	filename := sanitizeName(prefix + title + ext)
	return filepath.Join(o.libraryDir, sanitizeName(artist), sanitizeName(album), filename)
}

// trackPrefix produces the "01. " or "01-02. " prefix for a file name.
func trackPrefix(m *audioMeta) string {
	if m == nil || m.Track == 0 {
		return ""
	}
	// Pad width: use total if available, else default to 2 digits.
	trackWidth := 2
	if m.TrackTotal > 99 {
		trackWidth = 3
	}
	trackStr := fmt.Sprintf("%0*d", trackWidth, m.Track)

	// Include disc number only when there are multiple discs.
	if m.Disc > 0 && (m.DiscTotal > 1 || m.Disc > 1) {
		discWidth := 2
		if m.DiscTotal > 99 {
			discWidth = 3
		}
		discStr := fmt.Sprintf("%0*d", discWidth, m.Disc)
		return discStr + "-" + trackStr + ". "
	}
	return trackStr + ". "
}

// reservedNames are Windows-reserved file/directory names.
var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true,
	"COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true,
	"LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

var unsafeChars = regexp.MustCompile(`[<>:"/\\|?*` + "`" + `]`)

// sanitizeName makes a string safe to use as a file/directory name.
func sanitizeName(s string) string {
	// Replace filesystem-unsafe characters with underscores.
	s = unsafeChars.ReplaceAllString(s, "_")
	// Remove control characters.
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if s == "" {
		return "_"
	}
	// Avoid Windows-reserved names (case-insensitive, strip extension for check).
	base := strings.ToUpper(strings.TrimSuffix(s, filepath.Ext(s)))
	if reservedNames[base] {
		s = "_" + s
	}
	return s
}

// uniquePath appends a counter suffix to avoid overwriting an existing file.
func uniquePath(dest string) string {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest
	}
	ext := filepath.Ext(dest)
	base := strings.TrimSuffix(dest, ext)
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return dest
}

// moveFile moves src to dst, falling back to copy+delete across devices.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Cross-device move: copy then delete.
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := copyBuf(out, in); err != nil {
		os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		os.Remove(dst)
		return err
	}
	out.Close()
	return os.Remove(src)
}
