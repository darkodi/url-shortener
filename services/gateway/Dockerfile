# Stage 1: Builder
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build gateway binary — note: two source files in same package
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/gateway \
    ./services/gateway/main.go \
    ./services/gateway/proxy.go

# Stage 2: Runner
FROM alpine:latest

WORKDIR /app

RUN apk --no-cache add ca-certificates

COPY --from=builder /app/gateway .

# Gateway is the only public-facing service
EXPOSE 8080

ENTRYPOINT ["./gateway"]
