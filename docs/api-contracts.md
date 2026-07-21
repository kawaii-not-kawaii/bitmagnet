# API Contracts

## GraphQL API

Endpoint: POST /graphql (primary), GET /graphql (GraphQL playground)
Transport: HTTP, WebSocket (10s keepalive), multipart form
Cache: InMemory LRU 1000 + APQ LRU 100

## Queries

### version

```graphql
version: String!
```

Returns the compiled git tag version.

### workers

```graphql
workers: WorkersQuery!
workers {
  listAll {
    workers { key: String! started: Boolean! }
  }
}
```

### health

```graphql
health: HealthQuery!
health {
  status: HealthStatus!
  checks: [HealthCheck!]!
}
```

### torrent

```graphql
torrent {
  files(input: TorrentFilesQueryInput!): TorrentFilesQueryResult!
  listSources: TorrentListSourcesResult!
  suggestTags(input: SuggestTagsQueryInput): TorrentSuggestTagsResult!
  metrics(input: TorrentMetricsQueryInput!): TorrentMetricsQueryResult!
}
```

### torrentContent

```graphql
torrentContent {
  search(input: TorrentContentSearchQueryInput!): TorrentContentSearchResult!
}
```

### queue

```graphql
queue {
  jobs(input: QueueJobsQueryInput!): QueueJobsQueryResult!
  metrics(input: QueueMetricsQueryInput!): QueueMetricsQueryResult!
}
```

## Search Query Input

```graphql
input TorrentContentSearchQueryInput {
  queryString: String
  limit: Int
  page: Int
  offset: Int
  totalCount: Boolean
  hasNextPage: Boolean
  infoHashes: [Hash20!]
  facets: TorrentContentFacetsInput
  orderBy: [TorrentContentOrderByInput!]
  cached: Boolean
  aggregationBudget: Float
}
```

Available facets: contentType, torrentSource, torrentTag, torrentFileType, language, genre, releaseYear, videoResolution, videoSource

Search can be ordered by: relevance, published_at, updated_at, size, files_count, seeders, leechers, name, info_hash

## Search Result

```graphql
type TorrentContentSearchResult {
  totalCount: Int!
  totalCountIsEstimate: Boolean!
  hasNextPage: Boolean
  items: [TorrentContent!]!
  aggregations: TorrentContentAggregations!
}
```

## Mutations

### torrent

```graphql
torrent {
  delete(infoHashes: [Hash20!]!): Void
  putTags(infoHashes: [Hash20!]!, tagNames: [String!]!): Void
  setTags(infoHashes: [Hash20!]!, tagNames: [String!]!): Void
  deleteTags(infoHashes: [Hash20!], tagNames: [String!]): Void
  reprocess(input: TorrentReprocessInput!): Void
}

input TorrentReprocessInput {
  infoHashes: [Hash20!]!
  classifierRematch: Boolean
  classifierWorkflow: String
  apisDisabled: Boolean
  localSearchDisabled: Boolean
}
```

### queue

```graphql
queue {
  purgeJobs(input: QueuePurgeJobsInput!): Void
  enqueueReprocessTorrentsBatch(input: QueueEnqueueReprocessTorrentsBatchInput): Void
}

input QueueEnqueueReprocessTorrentsBatchInput {
  purge: Boolean
  batchSize: Int
  chunkSize: Int
  contentTypes: [ContentType]
  orphans: Boolean
  classifierRematch: Boolean
  classifierWorkflow: String
  apisDisabled: Boolean
  localSearchDisabled: Boolean
}
```

## Key Types

### TorrentContent

```graphql
type TorrentContent {
  id: ID!
  infoHash: Hash20!
  torrent: Torrent!
  contentType: ContentType
  contentSource: String
  contentId: String
  content: Content
  title: String!
  languages: [LanguageInfo!]
  episodes: Episodes
  videoResolution: VideoResolution
  videoSource: VideoSource
  videoCodec: VideoCodec
  video3d: Video3D
  videoModifier: VideoModifier
  releaseGroup: String
  seeders: Int
  leechers: Int
  publishedAt: DateTime!
  createdAt: DateTime!
  updatedAt: DateTime!
}
```

### Content

```graphql
type Content {
  type: ContentType!
  source: String!
  id: String!
  title: String!
  releaseDate: Date
  releaseYear: Year
  adult: Boolean
  originalLanguage: LanguageInfo
  originalTitle: String
  overview: String
  runtime: Int
  popularity: Float
  voteAverage: Float
  voteCount: Int
  attributes: [ContentAttribute!]!
  collections: [ContentCollection!]!
  metadataSource: MetadataSource!
  externalLinks: [ExternalLink!]!
}
```

### Torrent

```graphql
type Torrent {
  infoHash: Hash20!
  name: String!
  size: Int!
  hasFilesInfo: Boolean!
  singleFile: Boolean
  extension: String
  filesStatus: FilesStatus!
  filesCount: Int
  fileType: FileType
  fileTypes: [FileType!]
  files: [TorrentFile!]
  sources: [TorrentSourceInfo!]!
  seeders: Int
  leechers: Int
  tagNames: [String!]!
  magnetUri: String!
}
```

## Torznab API

Embedded torznab adapter for Servarr stack integration (Sonarr, Radarr, etc.):

- Newznab/Torznab API subset
- XML responses
- Capability reporting (categories, search modes)
- Category mapping across bitmagnet content types

## Import API

Endpoint: /import for ingesting torrent files from external sources (e.g., RARBG backup)

- HTTP endpoint
- Accepts torrent files or metadata

## Content Types

```graphql
enum ContentType {
  movie
  tv_show
  music
  ebook
  comic
  audiobook
  game
  software
  xxx
}
```

## File Types

```graphql
enum FileType {
  archive
  audio
  data
  document
  image
  software
  subtitles
  video
}
```

## Enums

- **FilesStatus:** no_info, single, multi, over_threshold
- **HealthStatus:** unknown, inactive, up, down
- **QueueJobStatus:** pending, retry, failed, processed
- **VideoResolution:** V360p, V480p, V540p, V576p, V720p, V1080p, V1440p, V2160p, V4320p
- **VideoSource:** CAM, TELESYNC, TELECINE, WORKPRINT, DVD, TV, WEBDL, WEBRip, BluRay
- **VideoCodec:** H264, x264, x265, XviD, DivX, MPEG2, MPEG4
- **Video3D:** V3D, V3DSBS, V3DOU
- **VideoModifier:** REGIONAL, SCREENER, RAWHD, BRDISK, REMUX
- **Language:** 64 ISO 639-1 language codes
- **MetricsBucketDuration:** minute, hour, day
