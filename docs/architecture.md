# Architecture

## Executive Summary

bitmagnet is a self-hosted BitTorrent indexer with a DHT crawler, content classifier, search engine, and web UI. It uses a multi-part architecture: a Go backend handles DHT crawling, classification, search, and a GraphQL API; an Angular SPA provides the user interface.

## System Architecture

The system is organized around a pipeline that starts with DHT network discovery and ends with indexed, classified torrent content available through the GraphQL API.

## Processing Pipeline

1. **DHT Crawler**: Discovers infohashes from the Kademlia DHT network using BEP 51 (sample_infohashes) and BEP 9 (metadata exchange)
2. **Persistence**: Raw torrent metadata is persisted to PostgreSQL
3. **Queue Job**: A delayed queue job is enqueued for each discovered torrent
4. **Processor**: Loads torrent data, runs the classifier, persists results
5. **Classifier**: YAML+CEL engine determines content type (movie, TV show, music, etc.), extracts attributes, attaches metadata from TMDB
6. **GraphQL API**: Searchable via rich faceted query with full-text search
7. **Web UI**: Angular SPA consumes the GraphQL API

### Architecture Diagram

```text
[DHT Network] <---> DHT Crawler
                      |
                      v (batched, persisted)
              [PostgreSQL Database]
                      |
                      v (queue job +1min delay)
              Processor -> Classifier -> TMDB
                      |
                      v (content + tags persisted)
              [PostgreSQL Database]
                      ^
                      |
              GraphQL API (gqlgen)
              /              \
             v                v
       Angular SPA        Torznab Adapter
       (webui/)           (Servarr stack)
```

## Backend Architecture

### Dependency Injection

The entire backend is composed using go.uber.org/fx. The main entry point creates an fx.App that aggregates all subsystem modules. Each subsystem exposes an fx module (e.g., dhtcrawlerfx, queuefx, processorfx) that provides its components.

Circular dependencies are resolved using lazy.Lazy[T] - a generic lazy evaluation wrapper.

22+ fx modules: appfx, dhtcrawlerfx, queuefx, processorfx, classifierfx, gqlfx, httpserverfx, databasefx, loggingfx, configfx, versionfx, telemetryfx, metricsfx, healthfx, blockingfx, validationfx, tmdb fx, torznab fx, importerfx, devfx, gqlfx, workerfx, various queue/processor sub-modules.

### Worker Registry

All background processes register as workers via a central Registry:

- `http_server` - Gin HTTP server
- `dht_crawler` - DHT crawling pipeline
- `queue_server` - Job queue polling server

The Registry enables/disables workers via CLI flags (--keys=http_server --keys=dht_crawler --keys=queue_server) or --all.

### HTTP Server

- Gin web framework on :3333
- Middleware: ginzap (request logging), recovery, CORS (permissive, all origins)
- Extensible via Option interface - each subsystem provides its handler routes
- CORS defaults allow all origins and headers (debug mode on)

### GraphQL API

- gqlgen v0.17.64 with code generation from .graphqls schema files
- 9 schema files covering queries, mutations, models, enums, scalars
- Transports: HTTP GET/POST, WebSocket (10s keepalive), multipart form
- Query caching: InMemory LRU 1000 + APQ cache LRU 100
- Custom scalars: Hash20, Date, DateTime, Duration, Void, Year
- 13 enums: ContentType, FileType, Language, VideoSource, etc.

### Configuration

Layered resolver chain with priority:

1. Extra YAML config files (EXTRA_CONFIG_FILES env)
2. Environment variables (e.g., HTTP_SERVER_LOCAL_ADDRESS)
3. ./config.yml (optional)
4. ~/.config/bitmagnet/config.yml (XDG)
5. Go struct defaults

Validated with go-playground/validator/v10 after resolution. Audit trail: ResolvedNode stores ResolverKey per field.

## Pipeline Architecture (detailed)

### DHT Crawler Pipeline

The crawler launches 14 concurrent goroutines connected by typed channels:

```text
reseedBootstrapNodes -> nodesForPing -> runPing
getOldNodes ----------> nodesForPing -> runPing

getNodesForFindNode --> nodesForFindNode --> runFindNode --> discoveredNodes
getNodesForSampleInfoHashes --> nodesForSampleInfoHashes --> runSampleInfoHashes --> infoHashTriage
                                                                  Bootstrap/Find responses --> discoveredNodes

discoveredNodes --> filter/route to:
  - nodesForFindNode
  - nodesForSampleInfoHashes
  - nodesForPing

infoHashTriage --> (DB lookup) --> getPeers (new/partial torrents)
                                --> scrape (known/outdated torrents)

getPeers --> requestMetaInfo --> persistTorrents
scrape   --> persistSources
persistTorrents --> also enqueues classify job (delayed 1min)
```

Stage details:

1. **Bootstrap**: DNS bootstrap node resolution, feeds into ping queue
2. **Node Discovery**: Deduplicates nodes by IP, routes to downstream channels
3. **Ping**: Verifies liveness, drops stale nodes
4. **Find Node**: Kademlia node lookup with rotating random target ID (10s rotation)
5. **Sample InfoHashes**: BEP 51 discovery through Stable Bloom Filter (10M capacity)
6. **InfoHash Triage**: DB lookup decides: fetch metadata, scrape, or discard
7. **Get Peers**: Requests peers for a specific infohash
8. **Request Meta Info**: BEP 9 metadata exchange via BitTorrent protocol
9. **Scrape**: BEP 33 seeders/leechers bloom filter request
10. **Persist**: Batched UPSERT to PostgreSQL

### Classifier Architecture

YAML-based classification engine:

1. Source provider merges core classifier + user config
2. Compiler parses YAML into an action tree using CEL expressions for conditions
3. Each workflow is a series of actions
4. Actions can attach content from TMDB via API or local DB search
5. Result includes content type, attributes, tags, and content reference

**Condition types**: and, or, not, expression (CEL)
**Action types**: set_content_type, add_tag, delete, parse_date, parse_video_content, if_else, find_match, run_workflow, attach_local_content_by_id/search, attach_tmdb_content_by_id/search

### Queue System

PostgreSQL-based job queue using SELECT FOR UPDATE SKIP LOCKED:

- Polling interval: 30s default (drops to 1ms when jobs available)
- Retry backoff: Sidekiq-inspired (retry^4 + 15 + random(30)\*retry + 1s)
- Panic-safe job execution with goroutine recovery
- Garbage collection: deletes processed/failed jobs every 10min

### Processor

Bridges the queue and the classifier:

1. Loads torrents from DB (with existing content matches)
2. For each torrent in parallel: runs classifier workflow
3. Re-queues failed hashes as new job
4. Persists results: Content, TorrentContent, TorrentTag, handles deletions

## Frontend Architecture

### Application Structure

- Standalone Angular 18 components with lazy loading
- No NgRx - simple BehaviorSubject + URL-driven state management
- Controller pattern: plain TypeScript classes own state and emit GraphQL variables
- DataSource pattern: CDK DataSource implementations for table-backed lists

### Key Design Decisions

- All GraphQL operations use fetchPolicy: "no-cache"
- Theme switching via CSS attribute on <html> element, persisted to localStorage
- 14 languages with static JSON imports (not lazy HTTP)
- Responsive design with CDK BreakpointObserver

## Integration Points

| Source  | Target   | Type           | Details                              |
| ------- | -------- | -------------- | ------------------------------------ |
| Web UI  | Backend  | GraphQL (HTTP) | All queries/mutations over HTTP POST |
| Backend | TMDB     | HTTP REST      | Content enrichment via TMDB API      |
| Backend | Postgres | SQL (pgx)      | All data persistence                 |
| Backend | Torznab  | Embedded       | Servarr stack integration            |

## Data Flow

### Torrent Ingestion Flow

```text
DHT -> Crawler -> PostgreSQL -> Queue -> Processor -> Classifier -> TMDB -> PostgreSQL -> GraphQL -> Web UI
```

### Search Flow

```text
User -> Angular -> GraphQL Query -> gqlgen -> Search Service -> PostgreSQL (GIN-indexed FTS) -> Response -> Web UI
```
