# ============================================================
# STAGE 1: Build
# ============================================================
FROM golang:1.23-alpine AS builder

# Install build dependencies (for SQLite CGO)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go mod files first (better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with CGO enabled (required for SQLite)
RUN CGO_ENABLED=1 GOOS=linux go build -o url-shortener ./cmd/server

# ============================================================
# STAGE 2: Runtime
# ============================================================
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite

# Create non-root user for security
RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -D appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/url-shortener .

# Create data directory for SQLite
RUN mkdir -p /app/data && chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./url-shortener"]
