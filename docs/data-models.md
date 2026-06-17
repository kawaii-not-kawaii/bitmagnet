# Data Models

## Database Schema Overview

PostgreSQL 16 database with GIN, GiST, and btree_gin extensions. Managed via goose migrations.

## Tables

### torrents
Primary repository for discovered torrent metadata.

| Column | Type | Constraints |
|--------|------|-------------|
| info_hash | bytea | PK, not null |
| name | text | not null |
| size | bigint | not null |
| private | boolean | not null |
| files_status | FilesStatus enum | not null, default no_info |
| extension | text | generated (single-file torrents) |
| files_count | integer | nullable |
| created_at | timestamptz | not null |
| updated_at | timestamptz | not null |

Relationships: has_many torrents_torrent_sources, torrent_files, torrent_contents, torrent_tags; has_one torrent_hints, torrent_pieces
Indexes: name, size, files_status, extension, created_at

### torrent_sources
External sources from which torrents were indexed.

| Column | Type | Constraints |
|--------|------|-------------|
| key | text | PK |
| name | text | not null |

Seed data: dht (DHT), rarbg (RARBG)

### torrents_torrent_sources
Join table linking torrents to sources with peer stats.

| Column | Type | Constraints |
|--------|------|-------------|
| source | text | PK, FK -> torrent_sources |
| info_hash | bytea | PK, FK -> torrents |
| import_id | text | nullable |
| seeders | integer | nullable |
| leechers | integer | nullable |
| published_at | timestamptz | nullable |

Indexes: info_hash, import_id, seeders, leechers, created_at, updated_at

### torrent_files
Individual files within multi-file torrents.

| Column | Type | Constraints |
|--------|------|-------------|
| info_hash | bytea | PK, FK -> torrents |
| index | integer | not null, unique(info_hash, index) |
| path | text | PK, not null |
| extension | text | generated (from path) |
| size | bigint | not null |

Indexes: size, extension

### torrent_contents
Classification result linking a torrent to content with parsed attributes.

| Column | Type | Constraints |
|--------|------|-------------|
| id | text | PK, generated (info_hash:content_type:content_source:content_id) |
| info_hash | bytea | not null, FK -> torrents |
| content_type | text | nullable, FK -> content |
| content_source | text | nullable, FK -> metadata_sources |
| content_id | text | nullable |
| languages | JSONB | nullable |
| episodes | JSONB | nullable |
| video_resolution/source/codec/3d/modifier | text | nullable |
| release_group | text | nullable |
| tsv | tsvector | nullable |
| seeders/leechers | integer | nullable |
| published_at | timestamptz | not null |
| size | bigint | not null default 0 |
| files_count | integer | nullable |

Check constraints: content_type is not null or content_id is null; content_source is null or content_id is not null
Indexes: content_type, content_source, content_id, seeders, leechers, published_at, size, composite gin(content_type, tsv), gin(content_type, languages)

### content
Metadata about recognized content (movies, TV shows, etc.).

| Column | Type | Constraints |
|--------|------|-------------|
| type | text | PK |
| source | text | PK, FK -> metadata_sources |
| id | text | PK |
| title | text | not null |
| release_date | date | nullable |
| release_year | integer | nullable |
| adult | boolean | nullable |
| original_language | text | nullable |
| overview | text | nullable |
| runtime | integer | nullable |
| popularity/tmdb_vote_average/tmdb_vote_count | float/bigint | nullable |

Relationships: has_many content_attributes, torrent_contents; many_to_many content_collections
Indexes: type, source, id, release_date, adult, popularity, gin(tsv)

### content_attributes
Key-value attributes for content (genres, etc.).

| Column | Type | Constraints |
|--------|------|-------------|
| content_type/source/id | text | PK, FK -> content |
| source | text | PK, FK -> metadata_sources |
| key | text | PK |
| value | text | not null |

### content_collections
Collections/groups content belongs to (franchises, series).

| Column | Type | Constraints |
|--------|------|-------------|
| type | text | PK |
| source | text | PK, FK -> metadata_sources |
| id | text | PK |
| name | text | not null |

### metadata_sources
Authoritative sources for content metadata.

Seed data: tmdb (TMDB), imdb (IMDb), tvdb (The TVDB)

### torrent_tags
User-applied tags for organizing torrents.

| Column | Type | Constraints |
|--------|------|-------------|
| info_hash | bytea | PK, FK -> torrents |
| name | text | PK, CHECK(^[a-z0-9]+(-[a-z0-9]+)*$) |

### torrent_hints
Pre-classification hints for torrents (before full processing).

Contains content_type, content_source, content_id, title, release_year, languages, episodes, video attributes.

### torrent_pieces
Optional storage of BitTorrent piece hashes.

### queue_jobs
PostgreSQL-backed job queue.

| Column | Type | Constraints |
|--------|------|-------------|
| id | text | PK, default gen_random_uuid() |
| fingerprint | text | not null, unique partial index (pending/retry) |
| queue | text | not null |
| status | queue_job_status enum | not null |
| payload | jsonb | not null |
| retries/max_retries | integer | not null |
| run_after | timestamptz | not null |
| priority | integer | not null default 0 |

Trigger: queue_announce_job (pg_notify on insert)
Indexes: queue+status, composite id+queue+status+priority+run_after, gin(queue, payload)

### bloom_filters
Persistent bloom filters stored as PostgreSQL large objects (oid).

### key_values
Generic key-value store for application state.

## Enums

- **ContentType:** movie, tv_show, music, ebook, comic, audiobook, game, software, xxx
- **FilesStatus:** no_info, single, multi, over_threshold
- **QueueJobStatus:** pending, processed, retry, failed
- **VideoResolution:** V360p..V4320p (9 values)
- **VideoSource:** CAM, TELESYNC, TELECINE, WORKPRINT, DVD, TV, WEBDL, WEBRip, BluRay
- **VideoCodec:** H264, x264, x265, XviD, DivX, MPEG2, MPEG4

## Custom Go Types

- **protocol.ID**: 20-byte SHA1 info hash (binary, primary key for torrents)
- **Year**: uint16 with SQL/JSON/GQL support
- **Date**: Custom date (year,month,day) with SQL/JSON/GQL support
- **Duration**: time.Duration scanning PostgreSQL interval
- **Language/Languages**: ISO 639-1 codes; Languages is map[Language]struct{}
- **Episodes**: season->episode mapping (e.g. S01E01-E03)
- **Tsvector**: Custom PostgreSQL tsvector type with weight support
- **Null types**: NullInt, NullString, NullBool, NullFloat32, NullUint etc.
- **Maybe[T]**: Generic optional value type

## Migrations

20 migrations total, from initial schema (00001) through bloom filter large object migration (00020). Key evolutionary steps:
- 00001: Initial schema (core tables, pg_trgm, generated search columns)
- 00002: FilesStatus enum migration
- 00006: Migrated from generated tsvector to programmatic
- 00007: torrent_hints table (moved attributes out of torrent_contents)
- 00012: Queue system
- 00017: Added seeders/leechers/published_at to torrent_contents
- 00020: Bloom filter large object storage
