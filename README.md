# prometheus-mcp

An [MCP](https://modelcontextprotocol.io/) server that exposes the Prometheus
HTTP API to MCP-compatible clients (Claude Desktop, IDEs, custom agents, …).

It wraps the [official Prometheus Go client](https://github.com/prometheus/client_golang)
and speaks MCP via the [official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk).

## Features

- Two transports: **stdio** (local) and **streamable HTTP** (remote).
- Configuration via flags, environment variables (`PROMETHEUS_MCP_*`) or YAML.
- Optional bearer-token or HTTP basic authentication against Prometheus.
- Read-only tools covering the common Prometheus endpoints.

### Tools

| Name                         | Description                                                  |
| ---------------------------- | ------------------------------------------------------------ |
| `prometheus_search`          | Rank metrics by relevance to a keyword / NL query.           |
| `prometheus_query`           | Evaluate an instant PromQL query.                            |
| `prometheus_query_range`     | Evaluate a PromQL query over a time range.                   |
| `prometheus_query_exemplars` | Query exemplars (e.g. trace IDs) over a time range.          |
| `prometheus_label_names`     | List label names in the TSDB.                                |
| `prometheus_label_values`    | List values for a given label.                               |
| `prometheus_series`          | Find series matching selectors.                              |
| `prometheus_targets`         | List scrape targets (active and/or dropped).                 |
| `prometheus_alerts`          | List firing and pending alerts.                              |
| `prometheus_alertmanagers`   | List discovered Alertmanager instances.                      |
| `prometheus_rules`           | List recording and alerting rule groups (optionally filtered).|
| `prometheus_metadata`        | Return metadata (type, help, unit) for metrics.              |
| `prometheus_tsdb_status`     | TSDB cardinality stats and top series/labels.                |
| `prometheus_wal_replay`      | Current WAL replay status.                                   |
| `prometheus_status_config`   | The currently loaded Prometheus configuration.               |
| `prometheus_status_flags`    | The flags Prometheus was launched with.                      |
| `prometheus_buildinfo`       | Return Prometheus server build info.                         |
| `prometheus_runtimeinfo`     | Return Prometheus server runtime info.                       |

### Metric search

`prometheus_search` is a BM25 index built in-process from `/api/v1/metadata` and
the `__name__` label values. It lets an MCP client discover relevant metrics from a keyword or
natural-language query without dumping the entire series catalogue into
context (e.g. `"http request latency"` → `http_request_duration_seconds`).
Partial metric-name prefixes match too (e.g. `"http_req"` →
`http_request_duration_seconds`), and results can be filtered by metric type.

Metrics that lack `HELP`/`TYPE` metadata never appear in
`/api/v1/metadata`; to keep those discoverable the index also pulls the distinct
metric names from the `__name__` label and indexes them by name. The index is
rebuilt periodically; control the cadence with `--search-refresh-interval`
(default `5m`, `0` disables).

## Bounded output

Because results are consumed by an LLM, tools that can return large payloads
are bounded by default so a single call cannot exhaust the client's context
window. Every bound can be raised or disabled (`0` = unlimited) per call, and
bounded responses include `total` / `returned` / `truncated` metadata (queries
additionally include a `stats` object) so callers know when data was dropped:

| Tool                         | Parameter(s)                                   | Default            |
| ---------------------------- | ---------------------------------------------- | ------------------ |
| `prometheus_query`           | `max_series`                                   | `100` series       |
| `prometheus_query_range`     | `max_series`, `max_samples_per_series`         | `50` / `100`       |
| `prometheus_series`          | `limit`                                        | `500`              |
| `prometheus_label_names`     | `limit`                                        | `500`              |
| `prometheus_label_values`    | `limit`                                        | `500`              |
| `prometheus_query_exemplars` | `limit`                                        | `500`              |
| `prometheus_targets`         | `limit` (per state), `state` filter            | `200`              |
| `prometheus_metadata`        | `limit` (when `metric` is empty)               | `100`              |

Responses are emitted as compact (non-indented) JSON to minimise token usage.

## Install

```sh
go install github.com/denysvitali/prometheus-mcp@latest
```

`go install` does not stamp a version, so such a binary reports itself to MCP
clients as `prometheus-mcp dev`. Release archives and images carry the real tag.

Or build from source:

```sh
git clone https://github.com/denysvitali/prometheus-mcp.git
cd prometheus-mcp
go build -o prometheus-mcp .
```

### Docker

Multi-arch images (`linux/amd64`, `linux/arm64`) are published to GHCR on every
push to `main` (as `:latest`) and for every `v*.*.*` tag:

```sh
docker run --rm -p 8080:8080 \
  -e PROMETHEUS_MCP_URL=https://prometheus.example.com \
  ghcr.io/denysvitali/prometheus-mcp:latest
```

## Usage

### stdio

```sh
prometheus-mcp stdio --url https://prometheus.example.com
```

Example Claude Desktop / IDE config:

```json
{
  "mcpServers": {
    "prometheus": {
      "command": "prometheus-mcp",
      "args": ["stdio"],
      "env": {
        "PROMETHEUS_MCP_URL": "https://prometheus.example.com",
        "PROMETHEUS_MCP_BEARER_TOKEN": "..."
      }
    }
  }
}
```

### HTTP

```sh
prometheus-mcp http \
  --url https://prometheus.example.com \
  --listen-address :8080 \
  --path /mcp
```

The server implements the MCP streamable HTTP transport on the configured path.
Use `--stateless` for load-balanced deployments that cannot maintain sticky
sessions.

## Configuration

All flags can be supplied via environment variables, using the prefix
`PROMETHEUS_MCP_` and replacing dots/dashes with underscores:

| Flag                         | Env var                                    |
| ---------------------------- | ------------------------------------------ |
| `--url`                      | `PROMETHEUS_MCP_URL`                       |
| `--bearer-token`             | `PROMETHEUS_MCP_BEARER_TOKEN`              |
| `--basic-auth-username`      | `PROMETHEUS_MCP_BASIC_AUTH_USERNAME`       |
| `--basic-auth-password`      | `PROMETHEUS_MCP_BASIC_AUTH_PASSWORD`       |
| `--tls-insecure-skip-verify` | `PROMETHEUS_MCP_TLS_INSECURE_SKIP_VERIFY`  |
| `--search-refresh-interval`  | `PROMETHEUS_MCP_SEARCH_REFRESH_INTERVAL`   |
| `--check-connection`         | `PROMETHEUS_MCP_CHECK_CONNECTION`          |
| `--log-level`                | `PROMETHEUS_MCP_LOG_LEVEL`                 |
| `--listen-address` (http)    | `PROMETHEUS_MCP_HTTP_LISTEN_ADDRESS`       |
| `--path` (http)              | `PROMETHEUS_MCP_HTTP_PATH`                 |
| `--stateless` (http)         | `PROMETHEUS_MCP_HTTP_STATELESS`            |

A YAML config file can also be used (`--config` or
`~/.prometheus-mcp.yaml`):

```yaml
url: https://prometheus.example.com
bearer-token: ey...
tls:
  insecure-skip-verify: false
basic-auth:
  username: ""
  password: ""
http:
  listen-address: :8080
  path: /mcp
search:
  refresh-interval: 5m
check-connection: false
log-level: info
```

## Development

The commands CI runs, in the order it runs them:

```sh
go vet ./...
go test -race ./...
go build ./...
golangci-lint run
```

`go test` verifies that no test leaks a goroutine (`goleak` in `TestMain`), so a
failure there is a real shutdown bug, not a flake.

### Layout

```
main.go                     entry point; delegates to cmd
cmd/                        flags, config decoding, one file per transport
  config.go                 every viper key -> Config, read exactly once
  runtime.go                the startup path both transports share
internal/prometheus/        authenticated Prometheus API client + transports
internal/search/            BM25 metric index (index/score) + refresher
internal/server/            MCP server; one file per group of tools
docs/adr/                   decisions worth keeping
```

Dependencies point one way: `cmd` -> `internal/server` -> {`internal/prometheus`,
`internal/search`}. The two inner packages know nothing about MCP, and
`internal/search` knows nothing about Prometheus beyond the API interface it
reads.

## License

MIT
