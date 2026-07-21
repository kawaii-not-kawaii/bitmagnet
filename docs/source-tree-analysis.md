# Source Tree Analysis

## Annotated Directory Tree

```
bitmagnet/
├── main.go              # Application entry point -> calls app.New().Run()
├── go.mod               # Go module: github.com/bitmagnet-io/bitmagnet (Go 1.23)
├── go.sum
│
├── internal/             # All Go backend source code
│   ├── app/              # Application composition via fx DI
│   │   ├── app.go        # fx.New() with appfx + loggingfx
│   │   ├── appfx/        # fx module aggregating all sub-modules
│   │   ├── cli/          # CLI argument parsing, hooks
│   │   └── cmd/          # CLI commands
│   │       ├── classifiercmd/  # classifier show/schema
│   │       ├── configcmd/      # config show
│   │       ├── processcmd/     # process torrent
│   │       ├── reprocesscmd/   # reprocess torrents
│   │       └── workercmd/      # worker run/list
│   │
│   ├── config/           # Configuration system
│   │   ├── config.go     # Reflect-based layered config resolution
│   │   ├── configfx/     # fx module for typed config injection
│   │   └── configresolver/ # YAML, env var resolvers
│   │
│   ├── httpserver/       # HTTP server (Gin-based)
│   │   ├── server.go     # Server lifecycle, middleware stack
│   │   ├── config.go     # Listen address, CORS, Gin mode
│   │   ├── cors/         # CORS middleware
│   │   ├── ginzap/       # Zap logging middleware
│   │   └── httpserverfx/ # fx module
│   │
│   ├── gql/              # GraphQL API
│   │   ├── gql.gen.go    # Generated code (716KB)
│   │   ├── gqlgen.yml    # Codegen config
│   │   ├── resolvers/    # Query/mutation resolvers
│   │   ├── gqlmodel/     # GraphQL model types
│   │   ├── config/       # Handler config (caching, transports)
│   │   ├── enums/        # Enum stringers + gen
│   │   ├── httpserver/   # GraphQL HTTP handler registration
│   │   └── gqlfx/        # fx module
│   │
│   ├── worker/           # Worker lifecycle registry
│   │   └── worker.go     # Worker interface, Registry with Start/Stop, decorator support
│   │
│   ├── dhtcrawler/       # DHT crawler - the core pipeline
│   │   ├── crawler.go    # 14-goroutine concurrent pipeline
│   │   ├── factory.go    # DI wiring, channel initialization
│   │   ├── config.go     # Scaling, bootstrap nodes, thresholds
│   │   ├── bootstrap.go  # DNS bootstrap node resolution
│   │   ├── ping.go       # Node ping/liveness
│   │   ├── find_node.go  # Kademlia find_node
│   │   ├── sample_infohashes.go  # BEP 51 sampling
│   │   ├── get_peers.go  # BEP get_peers
│   │   ├── scrape.go     # BEP 33 scrape
│   │   ├── request_meta_info.go  # BEP 9 metadata exchange
│   │   ├── infohash_triage.go    # Decision hub
│   │   ├── discovered_nodes.go   # Node routing
│   │   ├── persist.go   # Batch persistence + queue enqueue
│   │   ├── metrics/     # Prometheus metrics
│   │   └── dhtcrawlerfx/ # fx module
│   │
│   ├── classifier/       # YAML+CEL content classifier
│   │   ├── classifier.go # Compiler: YAML -> action tree
│   │   ├── classifier.core.yml  # Core classification workflows
│   │   ├── runner.go     # Executor
│   │   ├── source.go     # Source/provider config
│   │   ├── condition*.go  # Condition types (and, or, not, expression)
│   │   ├── action*.go     # Action types (set_content_type, add_tag, find_match, etc.)
│   │   ├── cel_env.go    # CEL environment setup
│   │   ├── cel_lists.go  # CEL extension functions
│   │   ├── flag*.go      # Runtime flag system
│   │   ├── classification/  # Result types
│   │   └── classifierfx/  # fx module
│   │
│   ├── processor/        # Queue job processor
│   │   ├── processor.go  # Process: load torrents -> classify -> persist
│   │   ├── factory.go    # DI wiring
│   │   ├── persist.go    # Transactional content persistence
│   │   ├── message.go    # Job message params
│   │   ├── batch/        # Batch reprocess message
│   │   └── queue/        # Queue handler registration
│   │
│   ├── queue/            # PostgreSQL-backed job queue
│   │   ├── server/       # Polling server with SKIP LOCKED
│   │   ├── handler/      # Job handler wrapper
│   │   ├── manager/      # Purge, enqueue operations
│   │   ├── prometheus/   # Queue depth metrics
│   │   └── queuefx/      # fx module
│   │
│   ├── database/         # Database layer
│   │   ├── gorm.go        # GORM setup, pgxpool
│   │   ├── dao/           # Generated DAOs (gorm.io/gen)
│   │   ├── query/         # GenericQuery engine, Criteria system, Facets
│   │   ├── search/        # Search service (TorrentContent, TorrentsWithMissingInfo, etc.)
│   │   ├── cache/         # Query cache (LRU)
│   │   ├── fts/           # Full-text search (tsvector, tsquery parser, tokenizer)
│   │   ├── exclause/      # SQL clause extensions (CTE, UNION)
│   │   ├── gen/           # GORM gen templates
│   │   └── databasefx/    # fx module
│   │
│   ├── model/            # Data models (GORM + custom types)
│   ├── protocol/         # BitTorrent protocol
│   │   ├── id.go         # 20-byte infohash
│   │   ├── int160.go     # 160-bit integer helpers
│   │   ├── dht/          # DHT protocol (Kademlia kTable, client, responder, KRPC)
│   │   └── metainfo/     # Torrent metainfo parsing
│   │
│   ├── torznab/          # Torznab API (Servarr integration)
│   ├── tmdb/             # TMDB API client
│   ├── importer/         # Torrent import facility
│   ├── health/           # Health check system
│   ├── logging/          # Logging (zap, file rotation)
│   ├── telemetry/        # Runtime observability
│   ├── metrics/          # Prometheus metric buckets
│   ├── concurrency/      # Typed channel primitives
│   ├── blocking/         # Blocklist manager
│   ├── bloom/            # Bloom filter utilities
│   ├── regex/            # Regex utilities
│   ├── slice/            # Slice utilities
│   ├── maps/             # Ordered maps
│   ├── keywords/         # Keyword parsing
│   ├── lexer/            # Tokenizer/lexer
│   ├── lazy/             # Lazy[T] for circular DI
│   ├── validation/       # Validator
│   ├── version/          # Version info
│   ├── protobuf/         # Protobuf definitions (Torrent, Classification)
│   └── dev/              # Development utilities (migrate, gorm gen)
│
├── webui/                # Angular frontend
│   ├── src/
│   │   ├── main.ts       # Bootstrap
│   │   ├── index.html
│   │   ├── styles.scss   # M3 theming
│   │   ├── app/
│   │   │   ├── app.config.ts   # Application config (Apollo, Transloco, Router)
│   │   │   ├── app.routes.ts   # Lazy route definitions
│   │   │   ├── app.module.ts   # Shared module barrel
│   │   │   ├── layout/         # App shell (toolbar, nav)
│   │   │   ├── torrents/       # Torrent search, browse, permalink
│   │   │   ├── dashboard/      # Admin dashboard
│   │   │   │   ├── queue/      # Queue management, visualization
│   │   │   │   └── torrents/   # Torrent metrics
│   │   │   ├── health/         # Health widgets
│   │   │   ├── graphql/        # GraphQL codegen + services
│   │   │   ├── i18n/           # Translations (14 languages)
│   │   │   ├── themes/         # Theme system (4 themes)
│   │   │   ├── pipes/          # Filesize, time-ago, int-estimate
│   │   │   ├── paginator/      # Custom paginator
│   │   │   ├── charting/       # Chart wrapper
│   │   │   ├── util/           # Utilities
│   │   │   └── ...
│   │   └── environments/       # Env config
│   ├── embed.go          # Go embed of built UI
│   └── package.json
│
├── graphql/              # GraphQL schema definitions (.graphqls files)
│   └── schema/
│       ├── query.graphqls
│       ├── mutation.graphqls
│       ├── models.graphqls
│       ├── torrent_content.graphqls
│       ├── torrent_files.graphqls
│       ├── queue.graphqls
│       ├── metrics.graphqls
│       ├── enums.graphqls
│       └── scalars.graphqls
│
├── migrations/           # PostgreSQL migrations (goose)
├── observability/        # Grafana, Loki, Prometheus, Pyroscope configs
├── bitmagnet.io/          # Jekyll documentation site
├── .github/workflows/    # CI/CD pipelines
├── Dockerfile            # Production Docker build
├── ci.Dockerfile         # Multi-arch CI Docker build
├── docker-compose.yml    # Full stack with VPN, observability
├── flake.nix             # Nix dev shell
├── Taskfile.yml          # Task runner (build, test, gen, lint)
└── .goreleaser.yml       # GoReleaser config
```

## Critical Directories

| Directory              | Purpose                                         |
| ---------------------- | ----------------------------------------------- |
| `internal/`            | All Go code organized by subsystem              |
| `internal/app/`        | Application composition, CLI commands           |
| `internal/dhtcrawler/` | Core DHT crawling pipeline (the killer feature) |
| `internal/classifier/` | YAML+CEL content classification engine          |
| `internal/processor/`  | Processing pipeline bridge                      |
| `internal/gql/`        | GraphQL API layer                               |
| `internal/database/`   | Database ORM, queries, search, full-text search |
| `internal/model/`      | Data models with custom types                   |
| `internal/protocol/`   | BitTorrent/DHT protocol implementations         |
| `internal/queue/`      | PostgreSQL-backed job queue                     |
| `internal/worker/`     | Worker lifecycle management                     |
| `webui/src/app/`       | Angular frontend application                    |
| `graphql/schema/`      | GraphQL schema definitions (source of truth)    |
| `migrations/`          | Database schema migrations                      |
| `observability/`       | Grafana dashboards, agent configs               |
| `.github/workflows/`   | CI/CD pipeline definitions                      |

## Entry Points

- **Backend:** `main.go` -> `internal/app/app.go` (fx DI) -> CLI commands or workers
- **Web UI:** `webui/src/main.ts` -> standalone Angular bootstrap
- **GraphQL Playground:** GET http://localhost:3333/graphql
- **API Endpoint:** POST http://localhost:3333/graphql
