# bitmagnet Documentation Index

## Project Overview

- **Type:** Multi-part monorepo with 2 parts
- **Primary Language:** Go (backend) + TypeScript (frontend)
- **Architecture:** DHT Crawler -> Processor -> Classifier -> GraphQL API -> Web UI

### Quick Reference

#### Backend (root)

- **Type:** Backend Service
- **Tech Stack:** Go 1.23, Gin, GraphQL (gqlgen), PostgreSQL (GORM), Prometheus
- **Entry Point:** main.go -> internal/app/app.go (fx DI)

#### Web UI (webui/)

- **Type:** Web Frontend
- **Tech Stack:** Angular 18, Angular Material, Apollo GraphQL, Transloco i18n, Chart.js
- **Root:** webui/

## Generated Documentation

- [Project Overview](./project-overview.md)
- [Architecture](./architecture.md)
- [Source Tree Analysis](./source-tree-analysis.md)
- [API Contracts](./api-contracts.md)
- [Data Models](./data-models.md)
- [Development Guide](./development-guide.md)
- [Deployment Guide](./deployment-guide.md)

## Existing Documentation

- [bitmagnet.io Documentation Site](../bitmagnet.io/index.md) - Jekyll-powered documentation website
- [bitmagnet.io FAQ](../bitmagnet.io/faq.md) - Frequently asked questions
- [bitmagnet.io Setup Guide](../bitmagnet.io/setup.md) - Installation instructions
- [bitmagnet.io External Resources](../bitmagnet.io/external-resources.md) - External references
- [GitHub Repository](https://github.com/bitmagnet-io/bitmagnet) - Source code and issues
- [Website](https://bitmagnet.io) - Project website

## Getting Started

### Prerequisites

- Go 1.23.6+
- Node.js 22+
- PostgreSQL 16

### Quick Start

```bash
# Set up database
createdb bitmagnet

# Install dependencies and run migrations
task install-webui
task migrate

# Run all workers
go run . worker run --all
```

### Access

- **Web UI:** http://localhost:3334 (dev mode)
- **GraphQL API:** POST http://localhost:3333/graphql
- **GraphQL Playground:** GET http://localhost:3333/graphql
- **Torznab Endpoint:** Embedded in HTTP server

## Project Status

Alpha. Ready for preview, but expect bugs and API/database schema changes before 1.0.
