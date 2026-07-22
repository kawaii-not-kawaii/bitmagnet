---
title: Endpoints
parent: Guides
layout: default
nav_order: 1
redirect_from:
  - /endpoints.html
---

# **bitmagnet** Endpoints

**bitmagnet** exposes functionality on a number of endpoints:

- `/` - Redirects to `/webui`
- `/webui` - Main web user interface
- `/graphql` - GraphQL API including the GraphiQL browser interface — **requires authentication** (see [Authentication]({% link setup/configuration.md %}#authentication))
- `/torznab/*` - Torznab API for integration compatible applications — **not** covered by the GraphQL API key; secure it separately (see [Authentication]({% link setup/configuration.md %}#authentication))
- `/import` - Import API for adding new content to the library (see [the importing guide](/guides/import.html))
- `/metrics` - Prometheus metrics (see [the observability guide](/guides/observability-telemetry.html))
- `/debug/pprof/*` - Go pprof profiling endpoints (see [the observability guide](/guides/observability-telemetry.html))
- `/status` - Health check/status endpoint

## GraphQL

The `/graphql` endpoint accepts authenticated GraphQL requests and provides the GraphiQL browser interface. Requests require the configured API key with admin access; see [Authentication]({% link setup/configuration.md %}#authentication).

### Updating configuration

`config.setSection` replaces a whole configuration section. For example:

```graphql
mutation {
  config {
    setSection(
      input: {
        key: "tmdb"
        value: { enabled: true, api_key: "your-tmdb-api-key" }
      }
    ) {
      section {
        key
        runtimeChangeable
        value
      }
      applied
    }
  }
}
```

The response contains the updated section with sensitive values redacted. `applied` is `LIVE_APPLY_AVAILABLE` when supported settings took effect immediately, or `RESTART_REQUIRED` when the change was persisted for the next restart. See [Runtime configuration API]({% link setup/configuration.md %}#runtime-configuration-api) for supported and restricted sections.

### Inspecting LLM classifications

`llm.events` returns recent LLM classification attempts newest first. The optional limit is capped by the 500-event in-memory buffer:

```graphql
query {
  llm {
    events(limit: 20) {
      timestamp
      infoHash
      torrentName
      provider
      durationMs
      outcome
      contentType
      title
      year
      season
      episode
      languages
      error
    }
  }
}
```

`llm.stats` combines process-lifetime counts, windowed latency and throughput, provider counts, current classifier utilization, and the pending classification queue. The window defaults to 15 minutes:

```graphql
query {
  llm {
    stats(windowMinutes: 15) {
      attempted
      matched
      unmatched
      errored
      skipped
      successRate
      perProvider {
        provider
        attempted
        matched
        unmatched
        errored
      }
      inFlight
      concurrency
      windowStart
      oldestBuffered
      windowAttempted
      latencyP50Ms
      latencyP95Ms
      throughputPerMinute
      queuePending
    }
  }
}
```

`oldestBuffered` is set when the 500-event buffer does not cover the full requested window. Both queries use the same API-key authentication as other GraphQL queries.
