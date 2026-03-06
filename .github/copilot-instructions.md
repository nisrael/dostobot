# GitHub Copilot Instructions for DoStoBot

## Project Overview

DoStoBot is a self-hosted music download manager written in **pure Go** (no Node.js/npm). It is deployed as a Docker container, provides a web UI to queue download URLs, extracts archives (ZIP/TAR/TAR.GZ/TAR.BZ2), reads audio metadata, and organises files into a structured music library.

## Repository Structure

```
main.go          HTTP server, routes, request handlers, config
queue.go         Thread-safe download queue with JSON persistence
downloader.go    HTTP download, archive extraction (ZIP/TAR)
organizer.go     Audio metadata reading (dhowden/tag), library directory layout
auth.go          authentik forwardAuth middleware (X-Authentik-Username header check)
util.go          Shared utility functions (e.g. copyBuf, isAudio)
templates/       Server-side rendered HTML (Go html/template, no JS framework)
dostobot_test.go All tests (single test file, package main)
Dockerfile       Multi-stage build: golang:alpine → alpine
docker-compose.yml  Traefik-ready deployment with authentik forwardauth labels
```

## Build, Test, and Lint

```bash
# Build
go build .

# Run all tests
go test ./...

# Vet
go vet ./...

# Format
gofmt -w .
```

## Code Style and Conventions

- Follow standard Go conventions: `gofmt`, `go vet`, idiomatic Go.
- All exported symbols must have Go doc comments.
- Error strings should be lowercase and not end with punctuation (Go convention).
- Log lines use the pattern `log.Printf("<package>: <message>")`, e.g. `log.Printf("organizer: %s -> %s", src, dest)`.
- Use `fmt.Errorf("…: %w", err)` for error wrapping.
- No Node.js, no npm, no JavaScript frameworks — UI is server-side rendered with `html/template`.
- Keep all tests in `dostobot_test.go` under `package main`.
- Use table-driven tests with a `cases` slice, iterating with `for _, c := range cases`.

## Authentication

- Authentication is **entirely delegated** to [authentik](https://goauthentik.io/) via Traefik's `forwardAuth` middleware.
- The app validates the `X-Authentik-Username` header as a defence-in-depth measure (`auth.go`).
- The `/health` endpoint is intentionally exempt from authentication.
- Never add credential storage (usernames/passwords) to the application.

## Library Organisation

- Audio files are placed at: `{LIBRARY_DIR}/{library}/{AlbumArtist}/{Album}/{NN. Title.ext}`
- When multiple discs are present: `{LIBRARY_DIR}/{library}/{AlbumArtist}/{Album}/{DD-NN. Title.ext}`
- The default library name is `"Alben"`.
- Valid library names match `^[a-zA-Z0-9_öäüÖÄÜß-]+$`.

## Environment Variables

| Variable       | Default      | Description                            |
|----------------|--------------|----------------------------------------|
| `PORT`         | `8080`       | HTTP port inside the container         |
| `LIBRARY_DIR`  | `/music`     | Destination music library directory    |
| `DOWNLOAD_DIR` | `/downloads` | Temporary download/extraction area     |
| `DATA_DIR`     | `/data`      | Queue state persistence directory      |

## Key Types and Interfaces

- `Config` — runtime config loaded from env vars (`main.go`)
- `Queue` — thread-safe queue with JSON persistence (`queue.go`)
- `QueueItem` / `ItemStatus` — individual download item and its status (`queue.go`)
- `Organizer` — moves audio files into the library (`organizer.go`)
- `audioMeta` — extracted audio metadata (`organizer.go`)
- `ForwardAuthMiddleware` — HTTP middleware enforcing the authentik header check (`auth.go`)

## Dependencies

- `github.com/dhowden/tag` — audio metadata reading (ID3, FLAC, Vorbis tags). This is the **only** external dependency. Avoid adding new dependencies unless absolutely necessary.

## Docker / Deployment

- Multi-stage Dockerfile: build stage uses `golang:alpine`, runtime uses `alpine`.
- `APP_UID`/`APP_GID` build args (default `1002`) set the runtime user.
- Requires a Traefik instance on the `proxy-net` network with a `websecure` entrypoint and an authentik Proxy Provider in "Forward auth (single application)" mode.
