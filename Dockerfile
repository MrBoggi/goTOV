# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src

# Download dependencies first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
ARG BUILD_VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags="-w -s -X github.com/MrBoggi/goTOV/internal/version.BuildVersion=${BUILD_VERSION}" \
    -o /goTOV ./cmd/gotov/main.go

# Runtime stage
FROM alpine:3.20

# Install curl for healthcheck and su-exec for privilege dropping
RUN apk add --no-cache curl su-exec

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata wget

# Create non-root user for security
RUN addgroup -S gotov && adduser -S -G gotov gotov

WORKDIR /app

# Create config and data directories and set ownership
RUN mkdir config data && chown gotov:gotov config data
COPY --chown=gotov:gotov config/config.example.yaml /app/config/config.yaml

# Copy static web files
COPY --chown=gotov:gotov cmd/static ./cmd/static

# Copy binary from builder
COPY --from=builder --chown=gotov:gotov /goTOV .

# Copy entrypoint script
COPY scripts/docker-entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Configuration
ENV GOTOV_SERVER_PORT=8085
EXPOSE ${GOTOV_SERVER_PORT}

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=30s --retries=3 \
    CMD curl --fail http://localhost:8085/health || exit 1

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./goTOV", "server"]
