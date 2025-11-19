# Docker Setup for goTØV

This guide explains how to build and run goTØV in Docker.

## Quick Start

### Using Docker directly

1. **Build the image:**
   ```bash
   docker build -t gotov:latest .
   ```

2. **Run the container:**
   ```bash
   docker run -d \
     --name gotov \
     -p 8085:8085 \
     -v $(pwd)/config/config.yaml:/app/config/config.yaml:ro \
     -v $(pwd)/data:/app/data \
     gotov:latest
   ```

3. **View logs:**
   ```bash
   docker logs -f gotov
   ```

### Using Docker Compose (Recommended for development)

1. **Start the application:**
   ```bash
   docker-compose up -d
   ```

2. **View logs:**
   ```bash
   docker-compose logs -f gotov
   ```

3. **Stop the application:**
   ```bash
   docker-compose down
   ```

### With Production Stack (TimescaleDB + Grafana)

```bash
docker-compose --profile production up -d
```

This will start:
- **goTØV** on `http://localhost:8085`
- **TimescaleDB** on `localhost:5432`
- **Grafana** on `http://localhost:3000` (admin/admin)

## GitHub Actions - Automatic Builds

The repository includes two automated workflows:

### 1. Docker Build & Push (`docker-build.yml`)

**Triggered on:**
- Push to `main` branch
- Push of version tags (`v*`)
- Pull requests to `main`

**Actions:**
- Builds Docker image using BuildKit
- Pushes to GitHub Container Registry (GHCR) on main/tags
- Generates SBOM (Software Bill of Materials)
- Uses layer caching for faster builds

**Image location:**
```
ghcr.io/MrBoggi/goTOV:latest
ghcr.io/MrBoggi/goTOV:v1.0.0
ghcr.io/MrBoggi/goTOV:main
```

### 2. Security Scan (`docker-security.yml`)

**Triggered on:**
- Push to `main` branch
- Version tags
- Pull requests
- Weekly schedule (Sundays at 2 AM UTC)

**Actions:**
- Scans image with Trivy for vulnerabilities
- Lints Dockerfile with Hadolint
- Reports findings to GitHub Security tab

## Configuration

Create a `config/config.yaml` file based on `config/config.example.yaml`:

```yaml
opcua:
  endpoint: "opc.tcp://your-plc:4840"
  
brewfather:
  user_id: "your-user-id"
  api_key: "your-api-key"
```

## Environment Variables

- `GOTOV_SERVER_PORT` - Server port (default: 8085)
- `GOTOV_CONFIG_PATH` - Config file path (default: config/config.yaml)

## Data Persistence

The container stores SQLite databases in `/app/data`. Mount this volume to persist data:

```bash
docker run -v $(pwd)/data:/app/data gotov:latest
```

## Health Checks

The container includes a health check that verifies the API is responding:

```bash
docker ps
# Look for "healthy" status
```

## Building for Specific Architectures

The GitHub Actions workflow automatically handles multi-arch builds with BuildKit.

For manual builds:

```bash
# Linux AMD64
docker build --platform linux/amd64 -t gotov:latest .

# Linux ARM64 (e.g., Apple Silicon, Raspberry Pi)
docker build --platform linux/arm64 -t gotov:latest .
```

## Troubleshooting

### Container exits immediately
Check logs: `docker logs gotov`

### Cannot connect to OPC UA endpoint
Ensure the endpoint is accessible from the container network and is in the config.

### Permission denied in data volume
The container runs as non-root user `gotov`. Ensure proper permissions:
```bash
chmod 755 data/
```

## Security Considerations

- Container runs as non-root user `gotov`
- Uses minimal alpine base image
- Includes security scanning in CI/CD
- Implements health checks
- Binary is stripped of debug symbols

## Pushing to Docker Hub

To push to Docker Hub instead of GHCR:

1. Update the `REGISTRY` and `IMAGE_NAME` in `.github/workflows/docker-build.yml`
2. Set Docker Hub credentials in repository secrets
3. Update the login step to use Docker Hub credentials

## Local Development

For development with live reload:

```bash
go run ./cmd/gotov server
```

Or build and run locally:

```bash
go build -o bin/gotov ./cmd/gotov/main.go
./bin/gotov server
```
