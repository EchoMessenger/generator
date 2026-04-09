# EchoMessenger Incident Detection Test Generator

A comprehensive Go utility for testing the EchoMessenger audit service's incident detection capabilities. Simulates 7 malicious and anomalous user behaviors (R1-R7) via WebSocket (Tinode protocol) to validate security detection rules.

## Overview

**Generator** automatically:
- Creates virtual test users and topics via Tinode API
- Simulates realistic and malicious messaging patterns
- Generates audit events for detection system validation
- Exports Prometheus metrics and JSON event logs
- Runs scenarios with configurable concurrency and rate limiting

## Quick Start

### Prerequisites

- Go 1.21+
- Docker (for containerized deployment)
- Access to EchoMessenger server (WebSocket + REST API)

### Local Development

1. **Clone and configure**:
   ```bash
   cd generator
   cp config.example.yaml config.yaml
   # Edit config.yaml with your server URL, users, scenarios
   ```

2. **Build**:
   ```bash
   make build
   # or: GOFLAGS=-mod=mod go build -o build/generator ./cmd
   ```

3. **Run**:
   ```bash
   ./build/generator -config config.yaml -log-level info
   ```

### Docker Deployment

1. **Build image**:
   ```bash
   docker build -t echo-generator .
   ```

2. **Run with docker-compose** (includes Prometheus + Grafana):
   ```bash
   docker-compose up
   # Generator: http://localhost:8080/metrics
   # Prometheus: http://localhost:9090
   # Grafana: http://localhost:3000 (admin/admin)
   ```

3. **Or run standalone**:
   ```bash
   docker run -v $(pwd)/config.yaml:/app/config.yaml \
     -p 8080:8080 \
     echo-generator \
     -config /app/config.yaml \
     -events-log /app/events/events.jsonl \
     -metrics :8080
   ```

## CLI Flags

```
Usage: generator [OPTIONS]

Options:
  -config FILE          Path to YAML configuration (default: config.yaml)
  -log-level LEVEL      Log level: debug, info, warn, error (default: info)
  -log-file FILE        Write logs to file (empty = stdout only)
  -events-log FILE      Write structured events to JSONL file
  -metrics ADDR         Prometheus metrics endpoint (e.g., :8080; empty = disabled)
  -scenarios NAMES      Run specific scenarios: all or comma-separated (e.g., brute_force,volume_anomaly)
  -dry-run              Log events but don't send to server
  -help                 Show this message
  -version              Show version
```

### Examples

```bash
# Run all scenarios with metrics
./build/generator -config config.yaml -metrics :8080 -events-log events.jsonl

# Run only R1 and R4
./build/generator -config config.yaml -scenarios brute_force,volume_anomaly

# Dry run to validate without sending
./build/generator -config config.yaml -dry-run -log-level debug

# List available scenarios
./build/generator -help
```

## Configuration (YAML)

### Server Section

```yaml
server:
  url: "wss://localhost:16060/v0/channels"  # WebSocket endpoint
  api_key: "your-api-key"                   # For REST API calls
  api_endpoint: "https://localhost:6060"    # For user provisioning
  timeout_seconds: 30                       # Connection timeout
```

### Generator Section

```yaml
generator:
  max_concurrency: 10                          # Max parallel goroutines
  rate_limit_per_second: 100                   # Global rate limit (0 = unlimited)
  dry_run: false                               # Validate without sending
  log_level: info                              # debug, info, warn, error
  log_output: stdout                           # stdout, file, both
  log_file: generator.log                      # Log file path
  events_log_file: events.jsonl                # Event audit trail (JSONL)
  metrics_addr: ":8080"                        # Prometheus endpoint
  selected_scenarios: "all"                    # Scenario selection
```

### Users Section

Each user is auto-provisioned at startup:

```yaml
users:
  - id: attacker                               # Internal ID
    login: attacker_user                       # Tinode username
    password: "SecurePassword123!@#"           # Tinode password
    description: "Attacker account"
```

### Scenarios Section

Enable/configure individual scenarios:

```yaml
scenarios:
  # R1: Brute force login attempts
  brute_force:
    enabled: true
    target_login: victim_user
    attempts: [wrong_pass_1, wrong_pass_2, ..., correct_pass]
    interval_ms: 500

  # R2: Concurrent sessions from same user
  concurrent_sessions:
    enabled: true
    user_id: attacker
    session_count: 4
    connection_interval_ms: 100

  # R3: Mass deletion of messages
  mass_delete:
    enabled: true
    user_id: attacker
    topic: "p2p-attacker-victim"
    delete_burst_count: 12
    delete_interval_ms: 100

  # R4: High-frequency message publishing
  volume_anomaly:
    enabled: true
    user_id: attacker
    messages_per_minute: 200
    duration_seconds: 120

  # R5: Enumeration (subscribe to restricted topics)
  enumeration:
    enabled: true
    user_id: attacker
    topic_ids: ["grpAAA", "grpBBB", "grpCCC"]
    subscription_interval_ms: 200

  # R6: Inactive account sudden activity
  inactive_account:
    enabled: true
    user_id: inactive
    message_burst_count: 50
    # NOTE: Requires manual pre-staging (no activity for 30+ days)

  # R7: Off-hours traffic
  off_hours:
    enabled: true
    user_id: attacker
    messages_per_minute: 50
    business_hours_start: "09:00"   # 24-hour format
    business_hours_end: "18:00"
    duration_seconds: 300
    # Runs outside business hours; use 'at' scheduler or cron

  # Baseline: Normal realistic messaging
  normal:
    enabled: true
    users: [victim, normal_user_1]
    duration_seconds: 300
    messages_per_minute: 5
```

## Scenarios (R1-R7 Detection Rules)

### R1: Brute Force Login
**Simulates**: Series of failed login attempts followed by successful login.  
**Detection trigger**: N failed attempts within M seconds.  
**Config**:
```yaml
brute_force:
  target_login: victim_user
  attempts: 15  # Number of wrong attempts
  interval_ms: 500
```

### R2: Concurrent Sessions
**Simulates**: Multiple simultaneous WebSocket connections from same user.  
**Detection trigger**: ≥ N concurrent sessions from single user_id.  
**Config**:
```yaml
concurrent_sessions:
  user_id: attacker
  session_count: 4
```
**Note**: Currently simulates from same client (IP spoofing deferred to Phase 5).

### R3: Mass Delete
**Simulates**: Rapid deletion of messages in a p2p topic.  
**Detection trigger**: N message deletions within M seconds.  
**Config**:
```yaml
mass_delete:
  user_id: attacker
  topic: "p2p-attacker-victim"
  delete_burst_count: 12
  delete_interval_ms: 100
```

### R4: Volume Anomaly
**Simulates**: Unusually high publishing rate from single user.  
**Detection trigger**: Messages/minute exceeds threshold.  
**Config**:
```yaml
volume_anomaly:
  user_id: attacker
  messages_per_minute: 200
  duration_seconds: 120
```

### R5: Enumeration
**Simulates**: Attempt to subscribe to closed/restricted topics.  
**Detection trigger**: Series of subscription attempts to inaccessible topics.  
**Config**:
```yaml
enumeration:
  user_id: attacker
  topic_ids: ["grpAAA", "grpBBB", "grpCCC"]  # Must be closed topics
```

### R6: Inactive Account Activity
**Simulates**: Sudden activity from dormant account.  
**Detection trigger**: Activity from account silent ≥ 30 days.  
**Config**:
```yaml
inactive_account:
  user_id: inactive
  message_burst_count: 50
```
**Setup**: Manually create `inactive` user and do not use it for 30+ days before running this scenario.

### R7: Off-Hours Activity
**Simulates**: Normal traffic outside configured business hours.  
**Detection trigger**: Significant activity outside 09:00-18:00.  
**Config**:
```yaml
off_hours:
  user_id: attacker
  messages_per_minute: 50
  business_hours_start: "09:00"
  business_hours_end: "18:00"
```
**Scheduling**: Use cron/scheduler to run outside business hours:
```bash
# Run at 22:00 (10 PM)
0 22 * * * /path/to/generator -config config.yaml -scenarios off_hours
```

### Baseline: Normal Traffic
**Simulates**: Realistic p2p and group messaging.  
**Detection trigger**: Should NOT trigger any detections.  
**Config**:
```yaml
normal:
  users: [victim, normal_user_1]
  duration_seconds: 300
  messages_per_minute: 5
```

## Monitoring & Metrics

### Prometheus Metrics

When `-metrics :8080` is set, metrics endpoint is available at `http://localhost:8080/metrics`:

```
# Counters
generator_messages_published_total{scenario}
generator_login_attempts_total{scenario, status}
generator_subscriptions_total{scenario}
generator_deletions_total{scenario}
generator_errors_total{scenario}

# Gauges
generator_active_connections
generator_active_scenarios
generator_message_queue_size{scenario}

# Histograms
generator_message_latency_seconds{scenario, le}
generator_scenario_duration_seconds{scenario, le}
generator_connection_latency_seconds{le}
```

### Event Logs (JSONL)

When `-events-log events.jsonl` is set, each event is logged as JSON:

```json
{"timestamp":"2024-01-15T14:30:00Z","event_type":"scenario_start","scenario":"brute_force","user":"attacker","status":"started","message":"Starting brute force scenario"}
{"timestamp":"2024-01-15T14:30:01Z","event_type":"login_attempt","scenario":"brute_force","user":"attacker","topic":"","status":"failed","message":"Wrong password"}
{"timestamp":"2024-01-15T14:30:01Z","event_type":"message_published","scenario":"volume_anomaly","user":"attacker","topic":"grp-general","status":"success","message":"Published message"}
```

### Logs (Structured)

Standard structured logging with timestamps, levels, and context:

```
2024-01-15T14:30:00Z INFO [brute_force] Starting scenario with 15 attempts
2024-01-15T14:30:01Z DEBUG [brute_force] Login attempt 1/15 (failed as expected)
2024-01-15T14:30:15Z INFO [brute_force] Scenario completed: 15 attempts, 14 failed, 1 success
```

## Troubleshooting

### "Connection refused" error
```
error: dial wss://localhost:16060: connection refused
```
**Solution**: Verify Tinode server is running and WebSocket endpoint is accessible.

### "Unauthorized: invalid credentials"
```
error: {ctrl code=401 text=unauthorized}
```
**Solution**: Check user credentials in config.yaml match those in Tinode server.

### "Topic not found"
```
error: {ctrl code=404 text=topic not found}
```
**Solution**: Ensure topics are created via provisioner or manually in Tinode before running scenarios.

### Metrics endpoint not responding
```
curl: (7) Failed to connect to localhost port 8080
```
**Solution**: Verify `-metrics :8080` flag was provided; check firewall rules.

### Dry run always succeeds
This is expected. Use `dry_run: true` to validate configuration without side effects.

## Performance Tuning

### High latency / timeouts

1. **Increase timeout**:
   ```yaml
   server:
     timeout_seconds: 60
   ```

2. **Reduce concurrency**:
   ```yaml
   generator:
     max_concurrency: 5
   ```

3. **Lower rate limit**:
   ```yaml
   generator:
     rate_limit_per_second: 50
   ```

### Memory usage growing

1. **Reduce log verbosity**:
   ```
   -log-level warn
   ```

2. **Disable events logging** (optional):
   ```
   # Don't set -events-log flag
   ```

3. **Reduce scenario duration**:
   ```yaml
   scenarios:
     volume_anomaly:
       duration_seconds: 60  # Was 120
   ```

## Architecture

### Components

```
cmd/main.go
  ├── Config loading (YAML)
  ├── User provisioning
  ├── Event logging setup
  ├── Metrics initialization
  └── Scenario execution

internal/client/
  ├── ws.go       → WebSocket connection management
  ├── session.go  → Session lifecycle (hi → login → token)
  └── actions.go  → High-level operations (Publish, Subscribe, Delete, etc.)

internal/scenario/
  ├── runner.go      → Scenario execution with concurrency control
  ├── normal.go      → Baseline and concurrent sessions
  └── malicious.go   → R1-R7 detection rule scenarios

internal/config/
  └── config.go      → YAML parsing and validation

internal/utils/
  ├── events.go      → Structured event logging
  └── metrics.go     → Prometheus metrics tracking
```

### Message Flow

```
{hi} (handshake)
  ↓
{login scheme="basic" secret=base64(user:pass)}
  ↓
{ctrl code=200 params={token}} ← authentication token
  ↓
{sub topic="..."} → subscribe to topic
  ↓
{pub content="..."}  → publish message
  ↓
{del what="msg" hard=false} → delete message
  ↓
{leave} → unsubscribe or logout
```

## Development

### Building

```bash
# Debug build with symbols
make build-debug

# Optimized build
make build

# All targets
make help
```

### Testing

```bash
# Unit tests
make test

# With coverage
make test-coverage

# Integration tests (requires running Tinode)
make test-integration
```

### Docker

```bash
# Build image
make docker-build

# Run with docker-compose
make docker-up

# View logs
make docker-logs

# Stop
make docker-down
```

## References

- [Tinode Server API Documentation](../../../docs/API.md)
- [WebSocket Protocol](https://datatracker.ietf.org/doc/html/rfc6455)
- [Prometheus Metrics Format](https://github.com/prometheus/docs/blob/main/content/docs/instrumenting/exposition_formats.md)

## License

Apache License 2.0

## Support

For issues, feature requests, or questions:
1. Check [Troubleshooting](#troubleshooting) section
2. Review [config.example.yaml](config.example.yaml) for configuration examples
3. Enable debug logging: `-log-level debug`
4. Check event logs: `-events-log events.jsonl` and examine JSON output

---

**Phase**: Production (4/4) | **Status**: Ready for deployment | **Last Updated**: 2024-01
