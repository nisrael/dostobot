package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// ── trackFromFilename ─────────────────────────────────────────────────────────

func TestTrackFromFilename(t *testing.T) {
	cases := []struct {
		name string
		want int
	}{
		{"03 - Song Title", 3},
		{"01. Song Title", 1},
		{"05 Example", 5},
		{"12-Title", 12},
		{"007_Bond Theme", 7},
		{"100 Track", 100},
		{"Song Title", 0},    // no leading number
		{"01", 0},            // number only, no separator
		{"not a track", 0},
	}
	for _, c := range cases {
		got := trackFromFilename(c.name)
		if got != c.want {
			t.Errorf("trackFromFilename(%q) = %d, want %d", c.name, got, c.want)
		}
	}
}

// ── sanitizeName ──────────────────────────────────────────────────────────────

func TestSanitizeName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Normal Album", "Normal Album"},
		{"AC/DC", "AC_DC"},
		{`Back\Slash`, "Back_Slash"},
		{"Com1", "_Com1"},              // Windows reserved
		{"", "_"},                      // empty → fallback
		{"  spaces  ", "spaces"},       // trim
		{"title: subtitle", "title_ subtitle"},
		{"\x00null", "null"}, // control chars stripped
	}
	for _, c := range cases {
		got := sanitizeName(c.in)
		if got != c.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── trackPrefix ──────────────────────────────────────────────────────────────

func TestTrackPrefix(t *testing.T) {
	cases := []struct {
		m    *audioMeta
		want string
	}{
		{nil, ""},
		{&audioMeta{}, ""},
		{&audioMeta{Track: 3}, "03. "},
		{&audioMeta{Track: 12, TrackTotal: 20}, "12. "},
		{&audioMeta{Track: 5, Disc: 2, DiscTotal: 2}, "02-05. "},
		{&audioMeta{Track: 1, Disc: 1, DiscTotal: 1}, "01. "}, // single disc
		{&audioMeta{Track: 100, TrackTotal: 100}, "100. "},
	}
	for _, c := range cases {
		got := trackPrefix(c.m)
		if got != c.want {
			t.Errorf("trackPrefix(%+v) = %q, want %q", c.m, got, c.want)
		}
	}
}

// ── destinationPath ───────────────────────────────────────────────────────────

func TestDestinationPath(t *testing.T) {
	org := newOrganizer("/music")

	cases := []struct {
		src     string
		meta    *audioMeta
		library string
		want    string
	}{
		{
			src: "/tmp/track.flac",
			meta: &audioMeta{
				AlbumArtist: "Pink Floyd",
				Album:       "The Wall",
				Title:       "Comfortably Numb",
				Track:       6,
			},
			library: "Alben",
			want:    "/music/Alben/Pink Floyd/The Wall/06. Comfortably Numb.flac",
		},
		{
			src: "/tmp/track.mp3",
			meta: &audioMeta{
				Artist: "Radiohead",
				Album:  "OK Computer",
				Title:  "Karma Police",
				Track:  3,
				Disc:   2,
				DiscTotal: 2,
			},
			library: "Alben",
			want:    "/music/Alben/Radiohead/OK Computer/02-03. Karma Police.mp3",
		},
		{
			// Track number inferred from filename ("03 - ...") when metadata has none.
			src: "/tmp/03 - Comfortably Numb.flac",
			meta: &audioMeta{
				AlbumArtist: "Pink Floyd",
				Album:       "The Wall",
				Title:       "Comfortably Numb",
				Track:       3,
			},
			library: "Alben",
			want:    "/music/Alben/Pink Floyd/The Wall/03. Comfortably Numb.flac",
		},
		{
			src:     "/tmp/song.flac",
			meta:    nil,
			library: "Alben",
			// no metadata – should use fallback names
			want: "/music/Alben/Unknown Artist/Unknown Album/song.flac",
		},
		{
			src:     "/tmp/song.flac",
			meta:    &audioMeta{}, // empty metadata
			library: "Alben",
			want:    "/music/Alben/Unknown Artist/Unknown Album/song.flac",
		},
		{
			src: "/tmp/track.flac",
			meta: &audioMeta{
				AlbumArtist: "Bach",
				Album:       "Goldberg Variations",
				Title:       "Aria",
				Track:       1,
			},
			library: "Klassik",
			want:    "/music/Klassik/Bach/Goldberg Variations/01. Aria.flac",
		},
	}

	for _, c := range cases {
		got := org.destinationPath(c.src, c.meta, c.library)
		if got != c.want {
			t.Errorf("destinationPath(%q, %+v, %q)\n  got  %q\n  want %q", c.src, c.meta, c.library, got, c.want)
		}
	}
}

// ── Queue ─────────────────────────────────────────────────────────────────────

func TestQueueAddAndGet(t *testing.T) {
	dir := t.TempDir()
	q := newQueue(filepath.Join(dir, "q.json"))

	q.add("https://example.com/a.zip", "Alben")
	q.add("https://example.com/b.zip", "Alben")

	items := q.getAll()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Status != StatusPending {
		t.Errorf("expected pending, got %s", items[0].Status)
	}
}

func TestQueueNextPending(t *testing.T) {
	dir := t.TempDir()
	q := newQueue(filepath.Join(dir, "q.json"))

	q.add("https://example.com/a.zip", "Alben")
	item := q.nextPending()
	if item == nil {
		t.Fatal("expected an item")
	}
	if item.Status != StatusDownloading {
		t.Errorf("expected downloading, got %s", item.Status)
	}
	// No more pending
	if q.nextPending() != nil {
		t.Error("expected nil after draining pending items")
	}
}

func TestQueueRemove(t *testing.T) {
	dir := t.TempDir()
	q := newQueue(filepath.Join(dir, "q.json"))

	item := q.add("https://example.com/a.zip", "Alben")
	q.remove(item.ID)
	if len(q.getAll()) != 0 {
		t.Error("expected empty queue after remove")
	}
}

func TestQueueRetry(t *testing.T) {
	dir := t.TempDir()
	q := newQueue(filepath.Join(dir, "q.json"))

	item := q.add("https://example.com/a.zip", "Alben")
	q.update(item.ID, StatusError, withError("timeout"))
	q.retry(item.ID)

	items := q.getAll()
	if items[0].Status != StatusPending {
		t.Errorf("expected pending after retry, got %s", items[0].Status)
	}
	if items[0].Error != "" {
		t.Errorf("expected error cleared after retry, got %q", items[0].Error)
	}
}

func TestQueuePersistence(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "q.json")

	q1 := newQueue(stateFile)
	q1.add("https://example.com/a.zip", "Alben")
	q1.add("https://example.com/b.zip", "Alben")

	// Load into a second queue instance
	q2 := newQueue(stateFile)
	if err := q2.load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(q2.getAll()) != 2 {
		t.Errorf("expected 2 persisted items, got %d", len(q2.getAll()))
	}
}

func TestQueueLoadResetsInProgress(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "q.json")

	q1 := newQueue(stateFile)
	item := q1.add("https://example.com/a.zip", "Alben")
	q1.update(item.ID, StatusDownloading)

	q2 := newQueue(stateFile)
	if err := q2.load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	items := q2.getAll()
	if items[0].Status != StatusPending {
		t.Errorf("expected in-progress item to be reset to pending, got %s", items[0].Status)
	}
}

// ── ForwardAuthMiddleware ─────────────────────────────────────────────────────

func TestForwardAuthMiddlewareForbiddenWithoutHeader(t *testing.T) {
	handler := ForwardAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden without auth header, got %d", rr.Code)
	}
}

func TestForwardAuthMiddlewareAllowedWithHeader(t *testing.T) {
	handler := ForwardAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Authentik-Username", "alice")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 OK with auth header, got %d", rr.Code)
	}
}

// ── uniquePath ────────────────────────────────────────────────────────────────

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "track.flac")

	// File doesn't exist – should return same path
	got := uniquePath(path)
	if got != path {
		t.Errorf("uniquePath(%q) = %q, want same path when file absent", path, got)
	}

	// Create the file, now should get a suffixed path
	f, _ := os.Create(path)
	f.Close()
	got2 := uniquePath(path)
	want2 := filepath.Join(dir, "track (2).flac")
	if got2 != want2 {
		t.Errorf("uniquePath with existing file = %q, want %q", got2, want2)
	}
}

// ── isAudio ───────────────────────────────────────────────────────────────────

func TestIsAudio(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"track.flac", true},
		{"song.mp3", true},
		{"music.M4A", true},
		{"cover.jpg", false},
		{"album.zip", false},
		{"readme.txt", false},
	}
	for _, c := range cases {
		got := isAudio(c.name)
		if got != c.want {
			t.Errorf("isAudio(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// ── validateLibrary ───────────────────────────────────────────────────────────

func TestValidateLibrary(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "Alben", false},             // empty → default
		{"Alben", "Alben", false},        // default value explicit
		{"Klassik", "Klassik", false},    // simple ASCII
		{"My-Library", "My-Library", false}, // hyphen allowed
		{"my_lib", "my_lib", false},      // underscore allowed
		{"öäüÖÄÜß", "öäüÖÄÜß", false},   // German umlauts allowed
		{"Lib123", "Lib123", false},      // digits allowed
		{"bad/name", "", true},           // slash not allowed
		{"bad name", "", true},           // space not allowed
		{"bad.name", "", true},           // dot not allowed
		{"../evil", "", true},            // path traversal
	}
	for _, c := range cases {
		got, err := validateLibrary(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("validateLibrary(%q): expected error, got %q", c.in, got)
			}
		} else {
			if err != nil {
				t.Errorf("validateLibrary(%q): unexpected error: %v", c.in, err)
			} else if got != c.want {
				t.Errorf("validateLibrary(%q) = %q, want %q", c.in, got, c.want)
			}
		}
	}
}
