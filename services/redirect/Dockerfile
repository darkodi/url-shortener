# Stage 1: Builder
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Leverage Docker layer cache — only re-downloads if go.mod/go.sum change
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the redirect service binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/redirect ./services/redirect/main.go

# Stage 2: Runner
FROM alpine:latest

WORKDIR /app

RUN apk --no-cache add ca-certificates

# Copy only the compiled binary from builder
COPY --from=builder /app/redirect .

# Redirect service listens on 8082
EXPOSE 8082

ENTRYPOINT ["./redirect"]
