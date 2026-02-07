# monitor-core

A high-performance event ingestion service that receives monitoring events via HTTP and writes them to ClickHouse in batches.

## Architecture

```
go services
  ↓ (batched NDJSON over HTTP)
monitor-core
  ↓ (batched inserts)
ClickHouse
```

## Features

- **HTTP ingestion endpoint**: `POST /v1/events` accepts NDJSON (newline-delimited JSON)
- **Gzip support**: Automatically handles gzip-compressed request bodies
- **Streaming parser**: Processes events line-by-line without loading entire body into memory
- **Batched writes**: Collects events and writes to ClickHouse in configurable batches
- **Non-blocking ingestion**: HTTP handler enqueues events and returns immediately
- **Simple API key authentication**: Via `X-Api-Key` header

## Quick Start

### 1. Start local ClickHouse

```bash
dev up
```

Or manually:

```bash
docker-compose -f docker-compose.dev.yml up -d
```

### 2. Run schema migrations

```bash
dev migrate
```

Or manually:

```bash
for f in migrations/*.sql; do clickhouse-client < "$f"; done
```

### 3. Configure environment (optional)

```bash
export API_KEY="your-secret-key"
```

All other defaults work with `dev up`.

### 4. Run the service

```bash
dev run
```

Or build and run:

```bash
go build -o bin/monitor-core .
./bin/monitor-core
```

### Docker (Production)

```bash
docker-compose up -d
```

## API

### Health Check

```bash
curl http://localhost:8080/health
```

Response:

```json
{ "status": "ok", "enqueued": 0, "dropped": 0, "pending": 0 }
```

### Ingest Events

```bash
curl -X POST http://localhost:8080/v1/events \
  -H "Content-Type: application/x-ndjson" \
  -H "X-Api-Key: your-secret-key" \
  -d '{"timestamp":"2026-02-06T23:01:02.123Z","service":"users","job_id":"job_x","request_id":"req_y","trace_id":"trc_z","name":"user.created","data":{"user_id":42}}
{"timestamp":"2026-02-06T23:01:02.456Z","service":"users","job_id":"job_x","request_id":"req_y","trace_id":"trc_z","name":"db.query","data":{"table":"users"}}'
```

Response:

```json
{ "accepted": 2 }
```

### Event Format

Each event must be a JSON object on its own line with these fields:

| Field        | Type             | Required | Description                                   |
| ------------ | ---------------- | -------- | --------------------------------------------- |
| `timestamp`  | string (RFC3339) | Yes      | When the event occurred                       |
| `service`    | string           | Yes      | Service name that generated the event         |
| `name`       | string           | Yes      | Event type/name                               |
| `env`        | string           | No       | Environment (e.g., production, staging)       |
| `job_id`     | string           | No       | Groups related requests within a service      |
| `request_id` | string           | No       | Unique identifier per incoming request        |
| `trace_id`   | string           | No       | Spans across services for distributed tracing |
| `user_id`    | string           | No       | User identifier for user-scoped queries       |
| `level`      | string           | No       | Log level (info, warn, error, debug)          |
| `data`       | object           | No       | Additional event data                         |

### Query Events

Query events with filters (Grafana-style):

```bash
curl "http://localhost:8080/v1/events?service=users&level=error&limit=50" \
  -H "X-Api-Key: your-secret-key"
```

**Query Parameters:**

| Parameter    | Description                                    |
| ------------ | ---------------------------------------------- |
| `service`    | Filter by service name                         |
| `env`        | Filter by environment                          |
| `job_id`     | Filter by job ID                               |
| `request_id` | Filter by request ID                           |
| `trace_id`   | Filter by trace ID                             |
| `user_id`    | Filter by user ID                              |
| `name`       | Filter by event name                           |
| `level`      | Filter by log level                            |
| `from`       | Start time (RFC3339 or Unix timestamp)         |
| `to`         | End time (RFC3339 or Unix timestamp)           |
| `data.<key>` | Filter by data field (e.g., `data.user_id=42`) |
| `limit`      | Results per page (default: 100, max: 1000)     |
| `offset`     | Pagination offset                              |

**Filter Operators:**

Filters support operators using Django-style syntax: `field__operator=value`

| Operator     | Example                          | Description             |
| ------------ | -------------------------------- | ----------------------- |
| `eq`         | `service=users` or `service__eq` | Equals (default)        |
| `neq`        | `level__neq=debug`               | Not equals              |
| `lt`         | `data.count__lt=100`             | Less than               |
| `gt`         | `data.count__gt=10`              | Greater than            |
| `lte`        | `data.latency__lte=500`          | Less than or equal      |
| `gte`        | `data.latency__gte=100`          | Greater than or equal   |
| `contains`   | `name__contains=user`            | Contains substring      |
| `startswith` | `service__startswith=auth`       | Starts with             |
| `endswith`   | `name__endswith=.error`          | Ends with               |
| `in`         | `level__in=error,warn`           | Matches any (comma-sep) |

**Examples:**

```bash
# Find errors and warnings
curl "http://localhost:8080/v1/events?level__in=error,warn"

# Find events with latency > 500ms
curl "http://localhost:8080/v1/events?data.latency_ms__gt=500"

# Find user-related events
curl "http://localhost:8080/v1/events?name__contains=user"

# Exclude debug logs
curl "http://localhost:8080/v1/events?level__neq=debug"
```

Response:

```json
{
  "success": true,
  "message": "request was successful",
  "pagination": { "count": 150, "next": "/v1/events?offset=100&limit=100", "previous": "" },
  "data": [{ "timestamp": "...", "service": "users", ... }]
}
```

### Label Autocomplete

Get distinct values for a label (service, env, name, level):

```bash
curl "http://localhost:8080/v1/labels/service/values" \
  -H "X-Api-Key: your-secret-key"
```

Response:

```json
{
  "success": true,
  "message": "request was successful",
  "data": ["users", "orders", "payments"]
}
```

### Data Keys Autocomplete

Get available keys from the `data` JSON column:

```bash
curl "http://localhost:8080/v1/data/keys?service=users" \
  -H "X-Api-Key: your-secret-key"
```

Response:

```json
{
  "success": true,
  "message": "request was successful",
  "data": ["client_ip", "host", "method", "path", "user_agent"]
}
```

### Data Values Autocomplete

Get values for a specific data key:

```bash
curl "http://localhost:8080/v1/data/values?key=method&service=users" \
  -H "X-Api-Key: your-secret-key"
```

Response:

```json
{
  "success": true,
  "message": "request was successful",
  "data": ["GET", "POST", "PUT", "DELETE"]
}
```

## Analytics API

The analytics API provides Grafana-compatible endpoints for building dashboards, charts, and gauges.

### Analytics Query

Aggregate data with optional grouping:

```bash
# Count events grouped by service
curl -X POST "http://localhost:8080/v1/analytics" \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: your-secret-key" \
  -d '{
    "aggregation": "count",
    "group_by": ["service"],
    "from": "2026-02-01T00:00:00Z",
    "to": "2026-02-06T23:59:59Z"
  }'
```

**Request Body:**

| Field         | Type     | Required | Description                                                   |
| ------------- | -------- | -------- | ------------------------------------------------------------- |
| `aggregation` | string   | No       | Aggregation type (default: `count`)                           |
| `field`       | string   | \*       | Field to aggregate (required for sum/avg/min/max/percentiles) |
| `group_by`    | string[] | No       | Fields to group by (max 10)                                   |
| `filters`     | object[] | No       | Filter conditions                                             |
| `from`        | string   | No       | Start time (RFC3339 or Unix)                                  |
| `to`          | string   | No       | End time (RFC3339 or Unix)                                    |
| `order_by`    | string   | No       | Field to order by (`value` or group field)                    |
| `order_desc`  | boolean  | No       | Order descending                                              |
| `limit`       | integer  | No       | Max results (default: 100, max: 10000)                        |

**Aggregation Types:**

| Type           | Description              | Requires Field |
| -------------- | ------------------------ | -------------- |
| `count`        | Count of events          | No             |
| `count_unique` | Count of unique values   | Yes            |
| `sum`          | Sum of numeric field     | Yes            |
| `avg`          | Average of numeric field | Yes            |
| `min`          | Minimum value            | Yes            |
| `max`          | Maximum value            | Yes            |
| `p50`          | 50th percentile          | Yes            |
| `p90`          | 90th percentile          | Yes            |
| `p95`          | 95th percentile          | Yes            |
| `p99`          | 99th percentile          | Yes            |

**Filter Format:**

```json
{
  "filters": [
    { "field": "service", "operator": "eq", "value": "users" },
    { "field": "data.status_code", "operator": "gte", "value": 400 }
  ]
}
```

Response:

```json
{
  "success": true,
  "data": {
    "data": [
      { "value": 1523, "groups": { "service": "users" } },
      { "value": 892, "groups": { "service": "orders" } }
    ],
    "total": 2
  }
}
```

**GET endpoint** (query-string based):

```bash
curl "http://localhost:8080/v1/analytics?aggregation=count&group_by=service&from=2026-02-01T00:00:00Z"
```

### Time Series Query

Get time-bucketed data for charts:

```bash
curl -X POST "http://localhost:8080/v1/timeseries" \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: your-secret-key" \
  -d '{
    "aggregation": "count",
    "interval": "hour",
    "filters": [{ "field": "name", "operator": "eq", "value": "user.login" }],
    "from": "2026-02-05T00:00:00Z",
    "to": "2026-02-06T23:59:59Z",
    "fill_zeros": true
  }'
```

**Request Body:**

| Field         | Type     | Required | Description                                  |
| ------------- | -------- | -------- | -------------------------------------------- |
| `aggregation` | string   | No       | Aggregation type (default: `count`)          |
| `field`       | string   | \*       | Field to aggregate                           |
| `interval`    | string   | Yes      | Time bucket size                             |
| `group_by`    | string[] | No       | Fields to group by (creates multiple series) |
| `filters`     | object[] | No       | Filter conditions                            |
| `from`        | string   | No       | Start time                                   |
| `to`          | string   | No       | End time                                     |
| `fill_zeros`  | boolean  | No       | Fill empty buckets with zero                 |

**Interval Types:** `minute`, `hour`, `day`, `week`, `month`

Response:

```json
{
  "success": true,
  "data": {
    "series": [
      {
        "name": "",
        "data_points": [
          { "timestamp": "2026-02-05T00:00:00Z", "value": 42 },
          { "timestamp": "2026-02-05T01:00:00Z", "value": 38 }
        ]
      }
    ]
  }
}
```

**GET endpoint:**

```bash
curl "http://localhost:8080/v1/timeseries?interval=hour&name=user.login&fill_zeros=true"
```

### Top N Query

Get top N values for a dimension:

```bash
curl -X POST "http://localhost:8080/v1/topn" \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: your-secret-key" \
  -d '{
    "aggregation": "count",
    "group_by": "data.endpoint",
    "limit": 10,
    "from": "2026-02-01T00:00:00Z",
    "to": "2026-02-06T23:59:59Z"
  }'
```

Response:

```json
{
  "success": true,
  "data": {
    "data": [
      { "key": "/api/users", "value": 5234 },
      { "key": "/api/orders", "value": 3891 }
    ]
  }
}
```

### Gauge Query

Get a single aggregated value:

```bash
curl -X POST "http://localhost:8080/v1/gauge" \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: your-secret-key" \
  -d '{
    "aggregation": "count",
    "filters": [{ "field": "level", "operator": "eq", "value": "error" }],
    "from": "2026-02-06T00:00:00Z",
    "to": "2026-02-06T23:59:59Z"
  }'
```

Response:

```json
{
  "success": true,
  "data": { "value": 127 }
}
```

### Compare Query

Compare current period with a previous period:

```bash
curl -X POST "http://localhost:8080/v1/compare" \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: your-secret-key" \
  -d '{
    "aggregation": "count",
    "filters": [{ "field": "name", "operator": "eq", "value": "http.request" }],
    "from": "2026-02-06T00:00:00Z",
    "to": "2026-02-06T23:59:59Z"
  }'
```

Response:

```json
{
  "success": true,
  "data": {
    "current": 1523,
    "previous": 1342,
    "change": 181,
    "change_percent": 13.49
  }
}
```

If `compare_from`/`compare_to` are not specified, the previous period is auto-calculated based on the duration of the current period.

## Configuration

| Environment Variable  | Default          | Description                                   |
| --------------------- | ---------------- | --------------------------------------------- |
| `HTTP_PORT`           | `8080`           | HTTP server port                              |
| `CLICKHOUSE_ADDR`     | `localhost:9000` | ClickHouse server address                     |
| `CLICKHOUSE_DATABASE` | `monitor`        | ClickHouse database name                      |
| `CLICKHOUSE_USERNAME` | `default`        | ClickHouse username                           |
| `CLICKHOUSE_PASSWORD` | ``               | ClickHouse password                           |
| `API_KEY`             | ``               | API key for authentication (empty = disabled) |
| `BATCH_SIZE`          | `1000`           | Number of events per batch insert             |
| `FLUSH_INTERVAL`      | `5s`             | Max time to wait before flushing batch        |
| `QUEUE_SIZE`          | `100000`         | Max events in memory queue                    |

## Limits

- **Request body size**: 10 MB for ingestion, 1 MB for analytics queries
- **Time series query**: Max 90 days range, max 10,000 data points
- **Analytics query**: Max 10,000 results, max 10 group by fields
- **Top N query**: Max 1,000 results
- **ClickHouse connection retry**: 10 attempts with linear backoff (1s, 2s, ... 10s)

## Development

Use the `dev` CLI for common tasks:

```bash
dev help                  # List available commands
dev up                    # Start local ClickHouse
dev migrate               # Run schema migrations
dev run                   # Run the application
dev check                 # Format, vet, and test
dev down                  # Stop local ClickHouse
```

## Project Structure

```
monitor-core/
  main.go                     # Entry point with routes
  Devfile.yaml                # Dev CLI commands
  Dockerfile                  # Multi-stage production build
  docker-compose.yml          # Production stack
  docker-compose.dev.yml      # Local development with ClickHouse
  db/
    clickhouse.go             # ClickHouse connection and batch writer
  env/
    env.go                    # Environment configuration
  middleware/
    auth.go                   # API key authentication middleware
    logging.go                # Request logging middleware
  responder/
    responder.go              # Standardized JSON response utilities
  routes/
    events.go                 # Event ingestion handler
    query.go                  # Event query and autocomplete handlers
    analytics.go              # Analytics, time series, and gauge handlers
  services/
    queue.go                  # Buffered event queue
    batcher.go                # Batch collection and flushing
    query.go                  # Query building and execution
    analytics.go              # Analytics query engine
  structs/
    event.go                  # Event struct and validation
    analytics.go              # Analytics query and result types
  migrations/
    001_schema.sql            # ClickHouse schema
    002_add_user_id.sql       # User ID column migration
```

## Querying Events

```sql
SELECT * FROM monitor.events LIMIT 10;

-- Find events by trace
SELECT * FROM monitor.events WHERE trace_id = 'trc_z';

-- Find events by service and time range
SELECT * FROM monitor.events
WHERE service = 'users'
  AND timestamp >= '2026-02-06 00:00:00';
```
