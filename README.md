# prometheus-mcp

An [MCP](https://modelcontextprotocol.io/) server that exposes the Prometheus
HTTP API to MCP-compatible clients (Claude Desktop, IDEs, custom agents, …).

It wraps the [official Prometheus Go client](https://github.com/prometheus/client_golang)
and speaks MCP via [mcp-go](https://github.com/mark3labs/mcp-go).

## Features

- Two transports: **stdio** (local) and **streamable HTTP** (remote).
- Configuration via flags, environment variables (`PROMETHEUS_MCP_*`) or YAML.
- Optional bearer-token or HTTP basic authentication against Prometheus.
- Read-only tools covering the common Prometheus endpoints.

### Tools

| Name                       | Description                                            |
| -------------------------- | ------------------------------------------------------ |
| `prometheus_query`         | Evaluate an instant PromQL query.                      |
| `prometheus_query_range`   | Evaluate a PromQL query over a time range.             |
| `prometheus_label_names`   | List label names in the TSDB.                          |
| `prometheus_label_values`  | List values for a given label.                         |
| `prometheus_series`        | Find series matching selectors.                        |
| `prometheus_targets`       | List scrape targets (active and dropped).              |
| `prometheus_alerts`        | List firing and pending alerts.                        |
| `prometheus_rules`         | List recording and alerting rule groups.               |
| `prometheus_metadata`      | Return metadata (type, help, unit) for metrics.        |
| `prometheus_buildinfo`     | Return Prometheus server build info.                   |
| `prometheus_runtimeinfo`   | Return Prometheus server runtime info.                 |

## Install

```sh
go install github.com/denysvitali/prometheus-mcp@latest
```

Or build from source:

```sh
git clone https://github.com/denysvitali/prometheus-mcp.git
cd prometheus-mcp
go build -o prometheus-mcp .
```

## Usage

### stdio

```sh
prometheus-mcp stdio --prometheus-url https://prometheus.example.com
```

Example Claude Desktop / IDE config:

```json
{
  "mcpServers": {
    "prometheus": {
      "command": "prometheus-mcp",
      "args": ["stdio"],
      "env": {
        "PROMETHEUS_MCP_PROMETHEUS_URL": "https://prometheus.example.com",
        "PROMETHEUS_MCP_PROMETHEUS_BEARER_TOKEN": "..."
      }
    }
  }
}
```

### HTTP

```sh
prometheus-mcp http \
  --prometheus-url https://prometheus.example.com \
  --listen-address :8080 \
  --path /mcp
```

The server implements the MCP streamable HTTP transport on the configured path.
Use `--stateless` for load-balanced deployments that cannot maintain sticky
sessions.

## Configuration

All flags can be supplied via environment variables, using the prefix
`PROMETHEUS_MCP_` and replacing dots/dashes with underscores:

| Flag                                    | Env var                                          |
| --------------------------------------- | ------------------------------------------------ |
| `--prometheus-url`                      | `PROMETHEUS_MCP_PROMETHEUS_URL`                  |
| `--prometheus-bearer-token`             | `PROMETHEUS_MCP_PROMETHEUS_BEARER_TOKEN`         |
| `--prometheus-basic-auth-username`      | `PROMETHEUS_MCP_PROMETHEUS_BASIC_AUTH_USERNAME`  |
| `--prometheus-basic-auth-password`      | `PROMETHEUS_MCP_PROMETHEUS_BASIC_AUTH_PASSWORD`  |
| `--prometheus-tls-insecure-skip-verify` | `PROMETHEUS_MCP_PROMETHEUS_TLS_INSECURE_SKIP_VERIFY` |
| `--log-level`                           | `PROMETHEUS_MCP_LOG_LEVEL`                       |
| `--listen-address` (http)               | `PROMETHEUS_MCP_HTTP_LISTEN_ADDRESS`             |
| `--path` (http)                         | `PROMETHEUS_MCP_HTTP_PATH`                       |
| `--stateless` (http)                    | `PROMETHEUS_MCP_HTTP_STATELESS`                  |

A YAML config file can also be used (`--config` or
`~/.prometheus-mcp.yaml`):

```yaml
prometheus:
  url: https://prometheus.example.com
  bearer-token: ey...
  tls:
    insecure-skip-verify: false
http:
  listen-address: :8080
  path: /mcp
log-level: info
```

## Development

```sh
go test ./...
go vet ./...
go build ./...
```

## License

MIT
