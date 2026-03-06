# dostobot
A Music Store Download Bot

DostoBot is a self-hosted music download manager deployed as a Docker container. It provides a clean web interface to submit download URLs (e.g. Qobuz purchase links), queues them, downloads and unpacks the archives, reads the audio metadata, and organises the files into your music library.

## Features

- **Web UI** – Submit download URLs from any browser; the queue updates automatically every 2 seconds (no page reload required)
- **Queue management** – Add, retry, and remove items; queue state is persisted to disk and survives restarts
- **Archive extraction** – ZIP, TAR, TAR.GZ, and TAR.BZ2 archives are unpacked automatically
- **Smart library organisation** – Reads ID3/FLAC/Vorbis tags via [`dhowden/tag`](https://github.com/dhowden/tag) and sorts files as:
  - `{Library}/{AlbumArtist}/{Album}/NN. Title.ext`
  - `{Library}/{AlbumArtist}/{Album}/DD-NN. Title.ext` (when multiple discs are present)
- **Authentication** – HTTP Basic Auth; password stored as a bcrypt hash
- **Traefik-ready** – Ships with Traefik labels for HTTPS (TLS termination by Traefik) and automatic HTTP→HTTPS redirect
- **No Node.js / npm** – Backend and UI are written in pure Go with server-side rendered HTML

![DostoBot UI](https://github.com/user-attachments/assets/5b899069-acb8-4ac7-992d-2900689399e1)

## Quick start

### 1. Generate a bcrypt password hash

```bash
docker run --rm -it alpine sh -c \
  'apk add --no-cache apache2-utils && htpasswd -bnBC 12 "" yourpassword | tr -d ":\n"'
```

### 2. Configure environment

```bash
cp .env.example .env
# Edit .env and set AUTH_PASSWORD_HASH, MUSIC_DIR, PUBLIC_HOST
```

### 3. Start

```bash
docker compose up -d
```

The service will be reachable at the domain you set in `PUBLIC_HOST` via HTTPS.

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP port the server listens on inside the container |
| `AUTH_USERNAME` | `admin` | Basic-auth username |
| `AUTH_PASSWORD_HASH` | *(required)* | bcrypt hash **or** plaintext password (hashed on startup) |
| `LIBRARY_DIR` | `/music` | Destination music library directory |
| `DOWNLOAD_DIR` | `/downloads` | Temporary download/extraction area |
| `DATA_DIR` | `/data` | Queue state persistence directory |

## Docker build arguments

| Argument | Default | Description |
|---|---|---|
| `APP_UID` | `1002` | UID for the `dostobot` runtime user |
| `APP_GID` | `1002` | GID for the `dostobot` runtime group |

Example: `docker build --build-arg APP_UID=1500 --build-arg APP_GID=1500 -t dostobot .`

## Library parameter

When adding a download URL via the web UI or the `POST /add` endpoint, an optional **Library** field is accepted (form field name: `library`).

- **Default:** `Alben`
- **Allowed characters:** `[a-zA-Z0-9_öäüÖÄÜß-]`
- The library name is used as the first directory inside `LIBRARY_DIR`, e.g. `LIBRARY_DIR/Alben/AlbumArtist/Album/…`

## Volumes

| Mount | Purpose |
|---|---|
| `MUSIC_DIR` → `/music` | Your existing audio library (host path) |
| `dostobot_data` → `/data` | Queue state JSON |
| `dostobot_downloads` → `/downloads` | Temporary files (auto-cleaned) |

## Development

```bash
go build .
PORT=8080 AUTH_PASSWORD_HASH=changeme LIBRARY_DIR=/tmp/music \
  DOWNLOAD_DIR=/tmp/downloads DATA_DIR=/tmp/data ./dostobot
```

Run tests:

```bash
go test ./...
```

## Architecture

```
main.go          HTTP server, routes, config
queue.go         Thread-safe download queue + JSON persistence
downloader.go    HTTP download, ZIP/TAR extraction
organizer.go     Audio metadata reading, library directory layout
auth.go          HTTP Basic Auth middleware (bcrypt)
templates/       Server-side rendered HTML (Go templates, no npm)
Dockerfile       Multi-stage build (golang:alpine → alpine)
docker-compose.yml  Traefik-ready deployment
```

