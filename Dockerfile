# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy Go modules files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application (CGO disabled for static binary)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o gh-cron-trigger main.go

# Runtime stage using distroless (minimal attack surface)
FROM gcr.io/distroless/static-debian12

# Copy the binary from builder stage
COPY --from=builder /app/gh-cron-trigger /app/gh-cron-trigger

# Copy configuration file
COPY config/config.yaml /app/config/

WORKDIR /app

# Expose the API port
EXPOSE 8080

# Default command
ENTRYPOINT ["./gh-cron-trigger"]
