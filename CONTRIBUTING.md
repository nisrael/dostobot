# Contributing to DoStoBot

Thank you for your interest in contributing! This document explains how to get
started, the workflow we use, and some notes on tooling — including GitHub
Copilot.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Code Style](#code-style)
- [Running Tests](#running-tests)
- [Submitting a Pull Request](#submitting-a-pull-request)
- [GitHub Copilot](#github-copilot)
- [Code of Conduct](#code-of-conduct)

---

## Getting Started

1. **Fork** the repository and clone your fork locally.
2. Make sure you have **Go 1.21+** and **Docker** installed.
3. Copy `.env.example` to `.env` and adjust the values for your environment.
4. Build and run locally:

   ```bash
   go build .
   PORT=8080 LIBRARY_DIR=/tmp/music DOWNLOAD_DIR=/tmp/downloads DATA_DIR=/tmp/data ./dostobot
   ```

   > Without Traefik + authentik in front of the app, every request returns
   > **403 Forbidden** because the `X-Authentik-Username` header is absent.
   > Use `curl -H "X-Authentik-Username: dev" http://localhost:8080/` for
   > quick local testing.

---

## Development Workflow

- Create a feature branch from `main`: `git checkout -b feat/my-feature`
- Keep commits small and focused.
- Write or update tests alongside your changes.
- Open a Pull Request against `main` when ready.

---

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Run `go vet ./...` before submitting.
- Keep exported symbols documented with Go doc comments.

---

## Running Tests

```bash
go test ./...
```

---

## Submitting a Pull Request

1. Ensure all tests pass (`go test ./...`).
2. Describe *what* changed and *why* in the PR description.
3. Reference any related issues with `Closes #<issue-number>`.
4. A maintainer will review and merge or request changes.

---

## GitHub Copilot

This project was developed with the assistance of **GitHub Copilot** and
welcomes its continued use by contributors. A few guidelines:

- **Review all Copilot suggestions** before committing — treat them as a
  starting point, not a finished answer.
- **Do not commit secrets or credentials**, even if Copilot suggests
  placeholder values.
- Copilot-assisted code is subject to the same review standards as
  hand-written code: correctness, tests, and style all matter.
- If you use Copilot Chat or agent mode, add a brief note in the PR
  description (e.g. *"Co-authored with GitHub Copilot"*) so reviewers have
  full context.

### Recommended VS Code settings for this project

```jsonc
// .vscode/settings.json (not committed — add to your local workspace)
{
  "github.copilot.enable": {
    "*": true,
    "go": true
  }
}
```

---

## Code of Conduct

Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md). We are
committed to making participation in this project a welcoming experience for
everyone.
