# Deployment Guide

## Docker Deployment

### Quick Start (Minimal)

```yaml
# docker-compose.yml (minimal)
services:
  bitmagnet:
    image: ghcr.io/bitmagnet-io/bitmagnet:latest
    ports:
      - "3333:3333"
    environment:
      - POSTGRES_HOST=postgres
      - POSTGRES_PASSWORD=postgres
    command:
      - worker
      - run
      - --all
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:16-alpine
    environment:
      - POSTGRES_PASSWORD=postgres
      - POSTGRES_DB=bitmagnet
    healthcheck:
      test: ["CMD-SHELL", "pg_isready"]
```

### Full Stack (with VPN + Observability)

A full-featured docker-compose.yml is included in the repository root with:

- **bitmagnet** - Main application (via gluetun VPN)
- **gluetun** - VPN container for DHT crawling
- **postgres** - Database (16-alpine)
- **grafana** - Dashboards and visualization
- **grafana-agent** - Metrics and log scraping
- **prometheus** - Metrics storage
- **loki** - Log aggregation
- **pyroscope** - Continuous profiling
- **postgres-exporter** - Database metrics

See `docker-compose.yml` for the complete configuration.

### Multi-Architecture Images

Docker images are built for: linux/386, amd64 (v2/v3), arm/v6, arm/v7, arm64, ppc64le, s390x
Published to: `ghcr.io/bitmagnet-io/bitmagnet`

## Configuration

Configuration is loaded from (in priority order):

1. Extra YAML files (EXTRA_CONFIG_FILES env)
2. Environment variables
3. ./config.yml (optional)
4. ~/.config/bitmagnet/config.yml (XDG)
5. Compiled defaults

### Key Environment Variables

| Variable                  | Description         | Default                      |
| ------------------------- | ------------------- | ---------------------------- |
| POSTGRES_HOST             | PostgreSQL host     | localhost                    |
| POSTGRES_PASSWORD         | Database password   | postgres                     |
| HTTP_SERVER_LOCAL_ADDRESS | Listen address      | :3333                        |
| TMDB_API_KEY              | TMDB API key        | (required for TMDB features) |
| LOG_FILE_ROTATOR_ENABLED  | Enable file logging | false                        |

## CI/CD Pipeline

GitHub Actions workflows:

### Checks (`checks.yml`)

- **lint** - ESLint, Prettier, golangci-lint
- **test** - Go tests + Angular tests
- **generated** - Verifies all generated code is up-to-date, runs migrations, builds web UI

### Container Registry (`ghcr.yml`)

- Triggers on v*.*.\* tags
- Multi-platform Docker build and push
- GoReleaser binary builds for all platforms
- Package formats: apk, deb, rpm, archlinux, tar.gz, zip

### Security (`codeql.yml`)

- CodeQL analysis for Go, JavaScript/TypeScript, Ruby
- Scheduled weekly and on PR to main

### Documentation (`jekyll-gh-pages.yml`)

- Builds and deploys Jekyll documentation site to GitHub Pages

## Release Process

1. Tag a commit: `git tag v0.1.0`
2. Push tag: `git push origin v0.1.0`
3. GitHub Actions builds:
   - Multi-arch Docker images to GHCR
   - Binary releases via GoReleaser
   - Packages: apk, deb, rpm, archlinux
   - Docker manifest list for all architectures

## Infrastructure

- **Container:** Alpine 3.20 base image (~40MB)
- **Database:** PostgreSQL 16 with pg_trgm and btree_gin extensions
- **Ports:** 3333 (HTTP/API), 3334 (BitTorrent TCP/UDP)
- **Volumes:** Config, data, and log directories

## Monitoring

The observability stack includes pre-configured:

- Grafana dashboards for system metrics
- Loki for log aggregation
- Prometheus for metric collection
- Pyroscope for continuous profiling

Configuration files are in `observability/`:

- `grafana-agent.config.river` - Agent pipeline
- `prometheus.config.yaml` - Metric collection
- `loki.config.yaml` - Log storage
- `pyroscope.config.yaml` - Profiling

## Recommended Hosting

- CPU: 2+ cores (DHT crawling is concurrent)
- RAM: 2GB+ (PostgreSQL + crawler + classifier)
- Storage: SSD recommended, grows with index size
- Network: Stable internet connection for DHT crawling
- VPN: Optional but recommended for DHT crawling
