# Multi-stage Dockerfile for HomeMate Server
# Stage 1: Build
FROM golang:1.23.6-alpine3.20 AS builder
WORKDIR /app
# Install build dependencies (CGO required for sqlite3)
RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev
# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download
# Copy source code
COPY . .
# Build the application (CGO_ENABLED=1 required for mattn/go-sqlite3)
RUN CGO_ENABLED=1 GOOS=linux go build -a -o homemate-server ./cmd/homemate

# Stage 2: Runtime
FROM alpine:3.20
WORKDIR /app
# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata curl
# Create non-root user
RUN addgroup -g 1000 -S homemate && \
    adduser -u 1000 -S homemate -G homemate
# Copy binary from builder
COPY --from=builder /app/homemate-server .
# Copy web assets
COPY --from=builder /app/web ./web
# Create upload directories
RUN mkdir -p uploads/health_records && chown -R homemate:homemate /app
# Switch to non-root user
USER homemate
# Expose port
EXPOSE 8080
# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/ || exit 1
# Run the application
ENTRYPOINT ["./homemate-server"]