package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

//go:embed templates
var templateFS embed.FS

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	Port             string
	AuthUsername     string
	AuthPasswordHash string
	LibraryDir       string
	DownloadDir      string
	DataDir          string
}

func loadConfig() Config {
	return Config{
		Port:             getEnv("PORT", "8080"),
		AuthUsername:     getEnv("AUTH_USERNAME", "admin"),
		AuthPasswordHash: getEnv("AUTH_PASSWORD_HASH", ""),
		LibraryDir:       getEnv("LIBRARY_DIR", "/music"),
		DownloadDir:      getEnv("DOWNLOAD_DIR", "/downloads"),
		DataDir:          getEnv("DATA_DIR", "/data"),
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// defaultLibrary is the library name used when none is specified.
const defaultLibrary = "Alben"

// libraryPattern matches valid library names.
var libraryPattern = regexp.MustCompile(`^[a-zA-Z0-9_öäüÖÄÜß-]+$`)

// validateLibrary returns the library name if valid, or an error.
func validateLibrary(lib string) (string, error) {
	if lib == "" {
		return defaultLibrary, nil
	}
	if !libraryPattern.MatchString(lib) {
		return "", fmt.Errorf("invalid library name: only letters, digits, _, - and öäüÖÄÜß are allowed")
	}
	return lib, nil
}

func main() {
	cfg := loadConfig()

	// Ensure required directories exist.
	for _, dir := range []string{cfg.LibraryDir, cfg.DownloadDir, cfg.DataDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("main: cannot create directory %s: %v", dir, err)
		}
	}

	// Set up queue with persistent state.
	q := newQueue(filepath.Join(cfg.DataDir, "queue.json"))
	if err := q.load(); err != nil {
		log.Printf("main: warning – could not load queue state: %v", err)
	}

	// Set up organizer and downloader.
	org := newOrganizer(cfg.LibraryDir)
	dl := newDownloader(cfg, q, org)
	go dl.run()

	// Set up authentication.
	auth := newAuth(cfg.AuthUsername, cfg.AuthPasswordHash)

	// Parse templates.
	tmpl := template.Must(template.New("").
		Funcs(template.FuncMap{
			"formatTime": func(t time.Time) string {
				if t.IsZero() {
					return ""
				}
				return t.Format("2006-01-02 15:04:05")
			},
			"statusClass": func(s ItemStatus) string {
				switch s {
				case StatusDone:
					return "status-done"
				case StatusError:
					return "status-error"
				case StatusPending:
					return "status-pending"
				default:
					return "status-active"
				}
			},
			"join": strings.Join,
		}).
		ParseFS(templateFS, "templates/*.html"))

	mux := http.NewServeMux()

	// GET / – main page
	mux.Handle("GET /", auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data := map[string]interface{}{
			"Items": q.getAll(),
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
			log.Printf("main: template error: %v", err)
		}
	})))

	// POST /add – enqueue a URL
	mux.Handle("POST /add", auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawURL := strings.TrimSpace(r.FormValue("url"))
		if rawURL == "" {
			http.Error(w, "url is required", http.StatusBadRequest)
			return
		}
		library, err := validateLibrary(strings.TrimSpace(r.FormValue("library")))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		q.add(rawURL, library)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})))

	// POST /retry/{id} – retry a failed item
	mux.Handle("POST /retry/", auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/retry/")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		q.retry(id)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})))

	// POST /delete/{id} – remove an item from the queue
	mux.Handle("POST /delete/", auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/delete/")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		q.remove(id)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})))

	// GET /health – unauthenticated health check (used by Docker/Traefik)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	})

	// GET /api/queue – JSON queue snapshot (for AJAX polling)
	mux.Handle("GET /api/queue", auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(q.getAll()); err != nil {
			log.Printf("main: json encode error: %v", err)
		}
	})))

	log.Printf("DostoBot listening on :%s  library=%s", cfg.Port, cfg.LibraryDir)
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
