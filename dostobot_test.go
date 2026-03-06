package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
		src  string
		meta *audioMeta
		want string
	}{
		{
			src: "/tmp/track.flac",
			meta: &audioMeta{
				AlbumArtist: "Pink Floyd",
				Album:       "The Wall",
				Title:       "Comfortably Numb",
				Track:       6,
			},
			want: "/music/Pink Floyd/The Wall/06. Comfortably Numb.flac",
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
			want: "/music/Radiohead/OK Computer/02-03. Karma Police.mp3",
		},
		{
			src:  "/tmp/song.flac",
			meta: nil,
			// no metadata – should use fallback names
			want: "/music/Unknown Artist/Unknown Album/song.flac",
		},
		{
			src: "/tmp/song.flac",
			meta: &audioMeta{}, // empty metadata
			want: "/music/Unknown Artist/Unknown Album/song.flac",
		},
	}

	for _, c := range cases {
		got := org.destinationPath(c.src, c.meta)
		if got != c.want {
			t.Errorf("destinationPath(%q, %+v)\n  got  %q\n  want %q", c.src, c.meta, got, c.want)
		}
	}
}

// ── Queue ─────────────────────────────────────────────────────────────────────

func TestQueueAddAndGet(t *testing.T) {
	dir := t.TempDir()
	q := newQueue(filepath.Join(dir, "q.json"))

	q.add("https://example.com/a.zip")
	q.add("https://example.com/b.zip")

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

	q.add("https://example.com/a.zip")
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

	item := q.add("https://example.com/a.zip")
	q.remove(item.ID)
	if len(q.getAll()) != 0 {
		t.Error("expected empty queue after remove")
	}
}

func TestQueueRetry(t *testing.T) {
	dir := t.TempDir()
	q := newQueue(filepath.Join(dir, "q.json"))

	item := q.add("https://example.com/a.zip")
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
	q1.add("https://example.com/a.zip")
	q1.add("https://example.com/b.zip")

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
	item := q1.add("https://example.com/a.zip")
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

// ── Auth ──────────────────────────────────────────────────────────────────────

func TestAuthCheck(t *testing.T) {
	a := newAuth("admin", "secret")

	if !a.check("admin", "secret") {
		t.Error("expected valid credentials to pass")
	}
	if a.check("admin", "wrong") {
		t.Error("expected wrong password to fail")
	}
	if a.check("other", "secret") {
		t.Error("expected wrong username to fail")
	}
}

func TestAuthBcryptHash(t *testing.T) {
	// newAuth with a plaintext value auto-hashes it; check should still verify.
	a := newAuth("user", "mypassword")
	if !a.check("user", "mypassword") {
		t.Error("expected auto-hashed password to verify correctly")
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
