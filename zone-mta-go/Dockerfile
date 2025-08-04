# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o zone-mta ./cmd/zone-mta

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Create app user
RUN addgroup -g 1001 app && \
    adduser -D -s /bin/sh -u 1001 -G app app

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /build/zone-mta .

# Copy configuration
COPY --from=builder /build/config ./config

# Create directories for data
RUN mkdir -p /app/data /app/logs && \
    chown -R app:app /app

# Switch to app user
USER app

# Expose ports
EXPOSE 2525 12080 9090

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:12080/health || exit 1

# Run the application
CMD ["./zone-mta", "-config", "config/default.yaml"]