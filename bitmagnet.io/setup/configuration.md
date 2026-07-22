---
title: Configuration
description: Configuration options for bitmagnet
parent: Setup
layout: default
nav_order: 2
---

# Configuration

{: .warning }
**Breaking change: the GraphQL API and web UI now require authentication.** After upgrading, **bitmagnet** requires an API key for the web UI and the GraphQL API. If you don't set one, a temporary key is generated at startup and printed to the log — so nothing hard-breaks, but you must supply that key (or set your own) to use the web UI. See [Authentication](#authentication) below.

**bitmagnet** exposes quite a few configuration options. You shouldn't have to worry about most of them, but they are available for tinkering. If you're using the [example docker-compose file]({% link setup/installation.md %}#docker) then things _should_ "just work". I'll only cover only some of the more important options here:

- `auth.api_key`: The API key required to access the GraphQL API and web UI. If left empty (and `auth.disabled` is `false`), a random key is generated at startup and logged — see [Authentication](#authentication).
- `auth.disabled` (default: `false`): Set to `true` to turn authentication off entirely. Only do this on a trusted network — it leaves the API and all mutations open to anyone who can reach the port.

- `postgres.host`, `postgres.name` `postgres.user` `postgres.password` (default: `localhost`, `bitmagnet`, `postgres`, _empty_): Set these values to configure connection to your Postgres database.
- `postgres.dsn`: Alternatively a Postgres Data Source Name (DSN) can be specified. If specified, all other `postgres.*` options are ignored.
- `tmdb.api_key`: This is quite an important one, please [see below](#obtaining-a-tmdb-api-key) for more details.
- `tmdb.enabled` (default: `true`): Specify `false` to disable the TMDB API integration.
- `dht_crawler.save_files_threshold` (default: `100`): Some torrents contain many thousands of files, which impacts performance and uses a lot of database disk space. This parameter sets a maximum limit for the number of files saved by the crawler with each torrent.
- `dht_crawler.save_pieces` (default: `false`): If true, the DHT crawler will save the pieces bytes from the torrent metadata. The pieces take up quite a lot of space, and aren't currently very useful, but they may be used by future features.
- `log.level` (default: `info`): If you're developing or just curious then you may want to set this to `debug`; note that `debug` output will be very verbose.
- `log.development` (default: `false`): If you're developing you may want to enable this flag to enable more verbose output such as stack traces.
- `log.json` (default: `false`): By default logs are output in a pretty format with colors; enable this flag if you'd prefer plain JSON.
- `log.file_rotator.enabled` (default: `false`): If true, logs will be output to rotating log files at level `log.file_rotator.level` in the `log.file_rotator.path` directory, allowing forwarding to a logs aggregator (see [the observability guide](/guides/observability-telemetry.html)).
- `http_server.options` (default `["*"]`): A list of enabled HTTP server components. By default all are enabled. Components include: `cors`, `pprof`, `graphql`, `import`, `prometheus`, `torznab`, `status`, `webui`.
- `dht_crawler.scaling_factor` (default: `10`): There are various rate and concurrency limits associated with the DHT crawler. This parameter is a rough proxy for resource usage of the crawler; concurrency and buffer size of the various pipeline channels are multiplied by this value. Diminishing returns may result from exceeding the default value of 10. Since the software has not been tested on a wide variety of hardware and network conditions your mileage may vary here...

To see a full list of available configuration options using the CLI, run:

```sh
bitmagnet config show
```

{% include callout_cli.md %}

For each configuration parameter available, this command will show:

- The path of the config key
- The Go type of the config key
- The currently configured value
- The default value
- Where the currently configured value has been sourced from (e.g. `default`, `./config.yml`, `env`)

## Specifying configuration values

Configuration paths are delimited by dots. If you're specifying configuration in a YAML file then each dot represents a nesting level, for example to configure `log.json`, `tmdb.api_key` and `http_server.cors.allowed_origins`:

```yaml
log:
  json: true
tmdb:
  api_key: my-api-key
http_server:
  cors:
    allowed_origins:
      - https://example1.com
      - https://example2.com
```

{: .note }
This is not a suggested configuration file, it's just an example of how to specify configuration values.

To configure these same values with environment variables, upper-case the path and replace all dots with underscores, for example:

```sh
LOG_JSON=true \
TMDB_API_KEY=my-api-key \
HTTP_SERVER_CORS_ALLOWED_ORIGINS=https://example1.com,https://example2.com \
  bitmagnet config show
```

## Configuration precedence

In order of precedence, configuration values will be read from:

- Environment variables
- The comma-separated list of config file paths specified in the `EXTRA_CONFIG_FILES` environment variable
- `config.yml` in the current working directory
- `config.yml` in the [XDG-compliant](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html) config location for the current user (for example on MacOS this is `~/Library/Application Support/bitmagnet/config.yml`)
- Default values

{: .warning }
Environment variables can be used to configure simple scalar types (strings, numbers, booleans) and slice types (arrays). For more complex configuration types such as maps you'll have to use YAML configuration. **bitmagnet** will exit with an error if it's unable to parse a provided configuration value.

## Authentication

The GraphQL API and the web UI require an API key. This protects not just the settings you can read, but every mutation — deleting torrents, purging queues, managing the blocklist — which were previously open to anyone who could reach the port.

Authentication is **enabled by default**. There is no configuration in which the API is silently open; you either supply a key, use a generated one, or explicitly disable authentication.

### Setting a key

Set `auth.api_key` to any value you like:

```yaml
auth:
  api_key: your-secret-key
```

or with an environment variable:

```sh
AUTH_API_KEY=your-secret-key
```

### If you don't set a key

If authentication is enabled and no `auth.api_key` is set, **bitmagnet** generates a random key at startup and logs it, for example:

```
No auth.api_key configured; generated a temporary GraphQL API key for this session: k7Fq...redacted...
Set auth.api_key in your config (or via the web UI) to make it permanent — the generated key changes on every restart
```

This means a fresh install works out of the box without leaving the API open — but the generated key **changes every time bitmagnet restarts**. Set `auth.api_key` to make it permanent.

### Using the web UI

When the web UI makes its first request to a server that requires a key, it prompts you to enter one. Paste the key from your config (or from the startup log) and it's stored in your browser. If authentication is disabled server-side, the web UI never prompts.

### Using the API directly

Send the key on every request in the `X-Api-Key` header (the same header the \*arr applications use):

```sh
curl -H "X-Api-Key: your-secret-key" http://localhost:3333/graphql ...
```

`Authorization: Bearer your-secret-key` is also accepted, if that fits your tooling better.

### Disabling authentication

On a trusted, isolated network you can turn authentication off:

```yaml
auth:
  disabled: true
```

{: .warning }
This leaves the GraphQL API **and all mutations** open to anyone who can reach the port. Only do this when the port is not otherwise exposed.

### Torznab is not covered

The `/torznab/*` endpoints are **not** affected by this setting — they remain unauthenticated so that Prowlarr and the \*arr applications continue to work with their own API-key scheme. If you expose Torznab, secure it by other means (a VPN, a reverse proxy, or firewall rules). Torznab-specific authentication is planned as a follow-up.

## VPN configuration

It's recommended that you run **bitmagnet** behind a VPN. If you're using Docker then [gluetun](https://github.com/qdm12/gluetun-wiki) is a good solution for this, although the networking settings can be tricky. The [example docker-compose file](https://github.com/bitmagnet-io/bitmagnet/blob/main/docker-compose.yml) demonstrates this.

## Obtaining a TMDB API Key

{: .highlight }
**bitmagnet** uses [the TMDB API](https://developer.themoviedb.org/docs) to fetch metadata for movies and TV shows. By default you'll be sharing an API key with other users. If you're using this app and its content classifier heavily then you'll need to get a personal TMDB API key. Until you do this you'll see a warning message in the logs on startup, and you'll be limited to 1 TMDB API request per second. This is just about enough for running the DHT crawler, but if you're importing and classifying a lot of content this will be a major bottleneck. If many people are using this app with the default API key then that could add up to many requests per second, so please get your own API key if you are using this app more than casually!

Obtaining an API key is free and relatively easy, but you'll have to register for a TMDB account, provide them with some personal information such as contact details, a website URL (such as your GitHub account or social media profile) and a short description of your use case (**tip:** this app provides _"A content classifier that identifies movies and TV shows based on filenames"_). Once you've filled in the request form, approval should be instant.

[Synology have provided a full tutorial on obtaining a TMDB API key](https://kb.synology.com/en-au/DSM/tutorial/How_to_apply_for_a_personal_API_key_to_get_video_info).

Once you've obtained your API key you'll need to configure the `tmdb.api_key` value. Your rate limit will then default to 20 requests per second, which is well within [TMDB's stated fair usage limit](https://developer.themoviedb.org/docs/rate-limiting).

{: .highlight }
The TMDB API integration can be disabled altogether by setting `tmdb.enabled` to `false`.
