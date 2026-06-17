# bitmagnet

A self-hosted BitTorrent indexer, DHT crawler, content classifier and torrent search engine with web UI, GraphQL API and Servarr stack integration.

- **Module:** github.com/bitmagnet-io/bitmagnet
- **License:** MIT
- **Repository:** github.com/bitmagnet-io/bitmagnet
- **Website:** https://bitmagnet.io
- **Status:** Alpha

## Architecture Type

Multi-part monorepo: Go backend server + Angular SPA frontend

## Repository Structure

| Part | Path | Type | Tech Stack |
|------|------|------|------------|
| Backend | `/` (root) | Backend API/Service | Go 1.23, Gin, GraphQL (gqlgen), PostgreSQL (GORM), Prometheus |
| Web UI | `webui/` | Web Frontend | Angular 18, Angular Material, Apollo GraphQL, Transloco i18n, Chart.js |
| Documentation Site | `bitmagnet.io/` | Static Site | Jekyll (Ruby), hosted on GitHub Pages |

## Tech Stack Summary

### Backend
- **Language:** Go 1.23.6
- **HTTP Framework:** Gin (gin-gonic/gin)
- **GraphQL:** gqlgen v0.17.64 with code generation
- **Database:** PostgreSQL via GORM with pgx driver, goose migrations
- **DI Framework:** go.uber.org/fx
- **CLI:** urfave/cli/v2
- **Config:** spf13/viper with layered YAML + env resolution
- **Logging:** go.uber.org/zap (structured JSON logs)
- **Metrics:** Prometheus client_golang
- **Caching:** gorm-cache with LRU (hashicorp/golang-lru/v2)
- **Full-text Search:** PostgreSQL tsvector with GIN indexes and custom tsquery parser
- **Concurrency:** Custom typed channels (BatchingChannel, BufferedConcurrentChannel)
- **Testing:** stretchr/testify, ginkgo/gomega, sqlmock
- **Build:** GoReleaser, Docker multi-arch builds
- **Profiling:** grafana/pyroscope

### Frontend
- **Framework:** Angular 18 (Standalone components, lazy loading)
- **UI Library:** Angular Material (v18) with M3 theming
- **GraphQL Client:** Apollo Angular with codegen
- **i18n:** @jsverse/transloco with 14 languages
- **Charts:** ng2-charts (Chart.js)
- **Testing:** Jasmine + Karma

### Infrastructure
- **Container:** Docker (Alpine-based), multi-arch image to GHCR
- **CI/CD:** GitHub Actions (lint, test, generated code check, multi-arch Docker build)
- **Release:** GoReleaser with nfpm packaging (apk, deb, rpm, archlinux)
- **Secrets:** .env file via godotenv/autoload
- **Observability:** Grafana dashboards, Loki logs, Prometheus metrics, Pyroscope profiling
- **VPN Integration:** Optional gluetun container for DHT crawling via VPN

## Key Features

- DHT crawler that discovers BitTorrent info hashes from the Kademlia DHT network
- BEP 9 metadata exchange to retrieve torrent metadata
- BEP 51 sample_infohashes for efficient hash discovery
- BEP 33 scrape for seeders/leechers
- YAML-driven content classifier with CEL expression conditions (9 content types)
- The Movie Database (TMDB) integration for content enrichment
- GraphQL API with rich search, faceted filtering, aggregations
- Torznab-compatible endpoint for Servarr stack integration (Sonarr, Radarr, etc.)
- Torrent import facility (e.g., RARBG backup import)
- Responsive Angular web UI with theme switcher and i18n
- Self-hosted, no external dependencies for core indexing

## High-Priority Missing Features

- Authentication / API keys / access levels
- Saved searches for custom feeds
- Bi-directional Prowlarr integration
