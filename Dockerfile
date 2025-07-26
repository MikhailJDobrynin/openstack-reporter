# Build stage
FROM golang:1.21-alpine AS builder

# Build arguments for version information
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

# Install git and build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with version information
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags "-X openstack-reporter/internal/version.Version=${VERSION} \
              -X openstack-reporter/internal/version.GitCommit=${GIT_COMMIT} \
              -X openstack-reporter/internal/version.BuildTime=${BUILD_TIME}" \
    -o openstack-reporter main.go

# Production stage
FROM alpine:latest

# Install ca-certificates, Python, and OpenStack CLI for multi-project support
RUN apk --no-cache add ca-certificates tzdata python3 py3-pip gcc musl-dev python3-dev linux-headers && \
    pip3 install --break-system-packages --no-cache-dir --no-compile python-openstackclient && \
    apk del py3-pip gcc musl-dev python3-dev linux-headers && \
    rm -rf /root/.cache /tmp/* && \
    ln -sf python3 /usr/bin/python

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/openstack-reporter .

# Copy web assets
COPY --from=builder /app/web ./web

# Create data directory
RUN mkdir -p data && \
    chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/status || exit 1

# Run the application
CMD ["./openstack-reporter"]
