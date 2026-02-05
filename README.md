# URL Shortener 🔗

A fast, lightweight URL shortener built with Go.

## Features

- ✅ Shorten long URLs
- ✅ Custom aliases
- ✅ Click statistics
- ✅ Rate limiting
- ✅ Input validation
- ✅ SQLite storage

---

## Quick Start

### Prerequisites

- Go 1.22+
- Docker & Docker Compose (for containerized setup)
- Make (optional, but recommended)

### Run Locally

Clone the repo:

    git clone https://github.com/darkodi/url-shortener.git
    cd url-shortener

Run directly:

    go run ./cmd/server

Or build and run:

    make build
    ./bin/url-shortener

### Run with Docker

One command to build and run:

    make up

Or manually:

    docker-compose up -d --build

App runs at: **http://localhost:8080**

---

## Makefile Commands

| Command | Description |
|---------|-------------|
| `make build` | Build binary to `./bin/` |
| `make run` | Run locally with `go run` |
| `make test` | Run all tests |
| `make test-cover` | Run tests with coverage report |
| `make clean` | Remove build artifacts |

### Docker Commands

| Command | Description |
|---------|-------------|
| `make docker-build` | Build Docker image |
| `make docker-run` | Start containers |
| `make docker-stop` | Stop containers |
| `make docker-logs` | Follow container logs |
| `make docker-clean` | Stop + remove volumes & images |
| `make up` | Build + Run (all-in-one) |
| `make down` | Stop everything |

---

## API Reference

### Create Short URL

    POST /shorten
    Content-Type: application/json

    {
      "url": "https://example.com/very/long/path",
      "custom_alias": "my-link"
    }

**Response:**

    {
      "short_url": "http://localhost:8080/abc123",
      "original_url": "https://example.com/very/long/path",
      "short_code": "abc123",
      "created_at": "2024-01-15T10:30:00Z"
    }

### Redirect

    GET /{short_code}

Example:

    curl -L http://localhost:8080/abc123

### Get Statistics

    GET /{short_code}/stats

**Response:**

    {
      "short_code": "abc123",
      "original_url": "https://example.com/very/long/path",
      "click_count": 42,
      "created_at": "2024-01-15T10:30:00Z"
    }

### Health Check

    GET /health

---

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_PORT` | `8080` | Server port |
| `DATABASE_PATH` | `urls.db` | SQLite database path |
| `APP_BASE_URL` | `http://localhost:8080` | Base URL for short links |
| `RATE_LIMIT_ENABLED` | `true` | Enable rate limiting |
| `RATE_LIMIT_RATE` | `10` | Requests per second |
| `RATE_LIMIT_BURST` | `20` | Burst limit |
| `LOG_LEVEL` | `info` | Log level (debug/info/warn/error) |
| `LOG_FORMAT` | `text` | Log format (text/json) |

---

## Project Structure

    url-shortener/
    ├── cmd/
    │   └── server/
    │       └── main.go           # Entry point
    ├── internal/
    │   ├── config/               # Configuration
    │   ├── errors/               # Error handling
    │   ├── handler/              # HTTP handlers
    │   ├── middleware/           # Rate limiting, logging
    │   ├── model/                # Data models
    │   ├── repository/           # Database layer
    │   ├── service/              # Business logic
    │   └── validator/            # Input validation
    ├── Dockerfile
    ├── docker-compose.yml
    ├── Makefile
    ├── go.mod
    ├── go.sum
    └── README.md

---

## Testing

Run all tests:

    make test

With coverage:

    make test-cover

View coverage report:

    open coverage.html

---

## License

MIT License
