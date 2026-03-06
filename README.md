# DoStoBot
A Music Store Download Bot

**DoStoBot** stands for **Do**wnload**Sto**re**Bot** — a self-hosted music download manager deployed as a Docker container. It provides a clean web interface to submit download URLs (e.g. Qobuz purchase links), queues them, downloads and unpacks the archives, reads the audio metadata, and organises the files into your music library.

## Features

- **Web UI** – Submit download URLs from any browser; the queue updates automatically every 2 seconds (no page reload required)
- **Queue management** – Add, retry, and remove items; queue state is persisted to disk and survives restarts
- **Archive extraction** – ZIP, TAR, TAR.GZ, and TAR.BZ2 archives are unpacked automatically
- **Smart library organisation** – Reads ID3/FLAC/Vorbis tags via [`dhowden/tag`](https://github.com/dhowden/tag) and sorts files as:
  - `{Library}/{AlbumArtist}/{Album}/NN. Title.ext`
  - `{Library}/{AlbumArtist}/{Album}/DD-NN. Title.ext` (when multiple discs are present)
- **Authentication via authentik** – All authentication is handled externally by [authentik](https://goauthentik.io/) via [Traefik's forwardAuth middleware](https://doc.traefik.io/traefik/middlewares/http/forwardauth/). No credentials are stored in the application.
- **Traefik-ready** – Ships with Traefik labels for HTTPS (TLS termination by Traefik), automatic HTTP→HTTPS redirect, and the authentik forwardauth middleware
- **No Node.js / npm** – Backend and UI are written in pure Go with server-side rendered HTML

![DoStoBot UI](https://github.com/user-attachments/assets/5b899069-acb8-4ac7-992d-2900689399e1)

## Quick start

### Prerequisites

- A running [Traefik](https://traefik.io/) instance on the `proxy-net` Docker network with a `websecure` entrypoint and a configured certificate resolver.
- A running [authentik](https://goauthentik.io/) instance with a **Proxy Provider** configured in **"Forward auth (single application)"** mode pointing at your DoStoBot domain.
  - See: [authentik Traefik forwardauth integration](https://goauthentik.io/docs/providers/proxy/forwardauth/)

### 1. Configure environment

```bash
cp .env.example .env
# Edit .env and set AUTHENTIK_HOST, PUBLIC_HOST, MUSIC_DIR
```

### 2. Start

```bash
docker compose up -d
```

The service will be reachable at the domain you set in `PUBLIC_HOST` via HTTPS.
All requests (except `/health`) require a valid authentik session — unauthenticated
requests are rejected by Traefik's forwardAuth middleware before they reach the app.
As a defence-in-depth measure, the application itself also returns **403 Forbidden**
if the `X-Authentik-Username` header is missing from the request.

## Authentication

Authentication is entirely delegated to authentik via Traefik's `forwardAuth` middleware.
There are no usernames or passwords stored inside DoStoBot.

### How it works

1. Traefik receives every incoming request.
2. Before forwarding to DoStoBot, Traefik sends the request to authentik's forward-auth endpoint (`/outpost.goauthentik.io/auth/traefik`).
3. If the session is valid, authentik returns 200 and injects headers such as `X-Authentik-Username` into the forwarded request.
4. If the session is invalid or absent, authentik returns a redirect to its login page (Traefik surfaces this to the browser).
5. DoStoBot validates the presence of `X-Authentik-Username` as a defence-in-depth check; requests without this header receive **403 Forbidden**.

The `/health` endpoint is intentionally exempt from authentication so Docker and Traefik can perform health checks.

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP port the server listens on inside the container |
| `LIBRARY_DIR` | `/music` | Destination music library directory |
| `DOWNLOAD_DIR` | `/downloads` | Temporary download/extraction area |
| `DATA_DIR` | `/data` | Queue state persistence directory |

> **Note:** `AUTH_USERNAME` and `AUTH_PASSWORD_HASH` have been removed. Authentication is now handled exclusively by authentik via Traefik's forwardAuth middleware. Set `AUTHENTIK_HOST` in your `.env` file to configure the forwardAuth address.

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
PORT=8080 LIBRARY_DIR=/tmp/music \
  DOWNLOAD_DIR=/tmp/downloads DATA_DIR=/tmp/data ./dostobot
```

> During local development without Traefik+authentik in front of the app, all
> requests will receive **403 Forbidden** because the `X-Authentik-Username`
> header is absent. You can temporarily set the header with a tool like `curl`:
> ```bash
> curl -H "X-Authentik-Username: dev" http://localhost:8080/
> ```

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
auth.go          authentik forwardAuth middleware (header validation)
templates/       Server-side rendered HTML (Go templates, no npm)
Dockerfile       Multi-stage build (golang:alpine → alpine)
docker-compose.yml  Traefik-ready deployment with authentik forwardauth
```

## GitHub Copilot

This project was developed with the assistance of **GitHub Copilot**. If you
contribute to DoStoBot, Copilot use is welcome — please see the
[CONTRIBUTING.md](CONTRIBUTING.md) guide for recommendations and best
practices specific to this project.

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for
details on the development workflow, code style, and how to submit a pull
request.

## License

This project is licensed under the [MIT License](LICENSE).

