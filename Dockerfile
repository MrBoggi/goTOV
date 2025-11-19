# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src

# Download dependencies first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags="-w -s" \
    -o /goTOV ./cmd/gotov/main.go

# Runtime stage
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user for security
RUN addgroup -S gotov && adduser -S -G gotov gotov

WORKDIR /app

# Copy binary from builder
COPY --from=builder --chown=gotov:gotov /goTOV .

# Switch to non-root user
USER gotov

# Configuration
ENV GOTOV_SERVER_PORT=8085
EXPOSE ${GOTOV_SERVER_PORT}

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${GOTOV_SERVER_PORT}/health || exit 1

ENTRYPOINT ["./goTOV"]
