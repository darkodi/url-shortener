# Stage 1: Builder
# Use a Go base image to compile the application
FROM golang:1.21-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
# This step only runs if go.mod or go.sum change
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
# This includes the 'internal' directory and the 'services/shortener' directory
COPY . .

# Build the shortener service binary
# CGO_ENABLED=0 is important for static compilation, resulting in a smaller image
# GOOS=linux ensures it's built for Linux
# The output binary is named 'shortener' and placed in /app
# We specify the path to the main package within the Go module
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/shortener ./services/shortener/main.go

# Stage 2: Runner
# Use a minimal Alpine Linux image to run the application
FROM alpine:latest

# Set the working directory for the final image
WORKDIR /app

# Install ca-certificates for HTTPS calls (if your service makes outgoing HTTPS requests)
RUN apk --no-cache add ca-certificates

# Copy the compiled binary from the builder stage into the final image
COPY --from=builder /app/shortener .

# Expose the port on which the shortener service will listen (e.g., 8081)
# This port will be mapped by docker-compose
EXPOSE 8081

# Define the command to run the application when the container starts
ENTRYPOINT ["./shortener"]
