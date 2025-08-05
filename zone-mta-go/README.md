# ZoneMTA-Go

A high-performance outbound SMTP relay (MTA/MSA) written in Go, ported from the original [ZoneMTA](https://github.com/zone-eu/zone-mta) Node.js project.

```
███████╗ ██████╗ ███╗   ██╗███████╗███╗   ███╗████████╗ █████╗     ██████╗  ██████╗ 
╚══███╔╝██╔═══██╗████╗  ██║██╔════╝████╗ ████║╚══██╔══╝██╔══██╗   ██╔════╝ ██╔═══██╗
  ███╔╝ ██║   ██║██╔██╗ ██║█████╗  ██╔████╔██║   ██║   ███████║   ██║  ███╗██║   ██║
 ███╔╝  ██║   ██║██║╚██╗██║██╔══╝  ██║╚██╔╝██║   ██║   ██╔══██║   ██║   ██║██║   ██║
███████╗╚██████╔╝██║ ╚████║███████╗██║ ╚═╝ ██║   ██║   ██║  ██║   ╚██████╔╝╚██████╔╝
╚══════╝ ╚═════╝ ╚═╝  ╚═══╝╚══════╝╚═╝     ╚═╝   ╚═╝   ╚═╝  ╚═╝    ╚═════╝  ╚═════╝ 
```

ZoneMTA-Go provides granular control over routing different messages through virtual "sending zones" with different IP addresses and reputation levels. It includes features like message queuing, retry logic, IP warm-up, plugin system, and HTTP API for programmatic access.

## Features

- **High Performance**: Built in Go for superior performance and memory efficiency
- **Sending Zones**: Route messages through different IP pools based on sender reputation
- **Reliable Queue**: MongoDB-based message queue with GridFS for large message storage
- **Retry Logic**: Intelligent retry with exponential backoff for temporary failures
- **Plugin System**: Extensible architecture with hook-based plugins
- **HTTP API**: RESTful API for message submission and queue management
- **Metrics**: Built-in monitoring and statistics
- **SMTP Server**: Full SMTP server implementation with STARTTLS support
- **Multiple Interfaces**: Support for multiple SMTP listening interfaces

## Requirements

- **Go** 1.21+ for building and running
- **MongoDB** 4.4+ for message queue storage
- **Redis** 6.0+ for locking and counters

## Quick Start

### 1. Clone and Build

```bash
git clone <repository-url> zone-mta-go
cd zone-mta-go
go build -o zone-mta ./cmd/zone-mta
```

### 2. Configure

Copy and edit the configuration file:

```bash
cp config/default.yaml config/zone-mta.yaml
# Edit config/zone-mta.yaml as needed
```

### 3. Start Services

Make sure MongoDB and Redis are running:

```bash
# MongoDB (default: localhost:27017)
mongod

# Redis (default: localhost:6379)
redis-server
```

### 4. Run ZoneMTA-Go

```bash
./zone-mta -config config/zone-mta.yaml
```

The server will start with:
- SMTP server on port 2525 (configurable)
- HTTP API on port 12080 (configurable)
- Metrics on port 9090 (configurable)

## Configuration

ZoneMTA-Go supports flexible multi-file YAML configuration with environment-specific overrides.

### Configuration Methods

#### Single File (Default)
```bash
./zone-mta -config config/default.yaml
```

#### Environment-Specific Loading
```bash
# Loads default.yaml + production.yaml
./zone-mta -env production

# Loads default.yaml + development.yaml
./zone-mta -env development
```

#### Directory-Based Loading
```bash
# Loads all YAML files from config/ directory with automatic prioritization
./zone-mta -config-dir config
```

#### Multiple Files
```bash
# Explicitly specify files to merge
./zone-mta -config-files "config/default.yaml,config/production.yaml,config/secrets.yaml"
```

### Configuration File Priority

When using directory or multi-file loading, files are loaded in this order:
1. `default.yaml` (base configuration)
2. Environment files (`production.yaml`, `development.yaml`, etc.)
3. Local overrides (`local.yaml`, `override.yaml`)
4. Secrets (`secrets.yaml`)

Later files override values from earlier files.

See `docs/CONFIGURATION.md` for detailed configuration documentation.

### Key Configuration Sections

#### Database Connections
```yaml
databases:
  mongodb:
    uri: "mongodb://localhost:27017/zone-mta-go"
    timeout: 10s
  redis:
    host: "localhost"
    port: 6379
    db: 3
```

#### SMTP Interfaces
```yaml
smtp:
  interfaces:
    feeder:
      enabled: true
      port: 2525
      host: "127.0.0.1"
      maxConnections: 100
```

#### Sending Zones
```yaml
sendingZones:
  default:
    connections: 5
    connectionTime: "10m"
    throttling: "100/hour"
  premium:
    connections: 10
    connectionTime: "5m"
    throttling: "500/hour"
```

## API Usage

### Send Email via HTTP API

```bash
curl -X POST http://localhost:12080/api/v1/queue/send \
  -H "Content-Type: application/json" \
  -d '{
    "from": "sender@example.com",
    "to": ["recipient@example.com"],
    "subject": "Test Email",
    "message": "From: sender@example.com\r\nTo: recipient@example.com\r\nSubject: Test Email\r\n\r\nHello World!",
    "zone": "default"
  }'
```

### List Messages

```bash
curl "http://localhost:12080/api/v1/queue/list?zone=default&limit=10"
```

### Get Queue Statistics

```bash
curl "http://localhost:12080/api/v1/queue/stats"
```

## SMTP Usage

Send email via SMTP:

```bash
telnet localhost 2525
HELO example.com
MAIL FROM:<sender@example.com>
RCPT TO:<recipient@example.com>
DATA
From: sender@example.com
To: recipient@example.com
Subject: Test Email

Hello World!
.
QUIT
```

## Plugin System

ZoneMTA-Go supports a plugin system for extending functionality:

```go
type Plugin interface {
    Name() string
    Description() string
    Version() string
    Initialize(ctx context.Context, config map[string]interface{}) error
    RegisterHooks(manager *Manager) error
    Shutdown(ctx context.Context) error
}
```

### Built-in Plugins

- **default-headers**: Adds missing headers like Message-ID and Date
- **message-logger**: Logs message events for debugging and monitoring
- **rate-limiter**: Implements rate limiting based on sender address and IP address

### Plugin Examples

The `plugins/` directory contains example plugins:

- **Rate Limiter Plugin** (`plugins/rate-limiter/`): Advanced example showing Redis integration for rate limiting
- **Message Logger Plugin** (`plugins/message-logger/`): Simple example for logging message events

See `examples/plugin-example/` for a complete demonstration of plugin development:

```bash
cd examples/plugin-example
go run main.go
```

### Metrics Example

Test the Prometheus metrics server:

```bash
cd examples/metrics-example
go run main.go
```

This will start a metrics server on port 9090 with simulated data to demonstrate the available metrics.

## Docker Support

### Using Docker

```bash
# Build image
docker build -t zone-mta-go .

# Run with docker-compose
docker-compose up -d
```

### Docker Compose

```yaml
version: '3.8'
services:
  zone-mta-go:
    build: .
    ports:
      - "2525:2525"
      - "12080:12080"
    depends_on:
      - mongodb
      - redis
    volumes:
      - ./config:/app/config

  mongodb:
    image: mongo:7
    volumes:
      - mongodb_data:/data/db

  redis:
    image: redis:7-alpine
    
volumes:
  mongodb_data:
```

## Performance

ZoneMTA-Go is designed for high performance:

- **Concurrent Processing**: Multiple worker goroutines per sending zone
- **Connection Pooling**: Efficient SMTP connection reuse
- **Message Batching**: Optimized database operations
- **Memory Efficient**: Streaming message content from GridFS

## Monitoring

### Health Check

```bash
curl http://localhost:12080/health
```

### Prometheus Metrics

ZoneMTA-Go includes a dedicated Prometheus metrics server on port 9090:

```bash
# Check metrics server health
curl http://localhost:9090/health

# Get Prometheus metrics
curl http://localhost:9090/metrics

# View ZoneMTA-specific metrics
curl http://localhost:9090/metrics | grep zonemta_
```

#### Available Metrics

- **Mail Processing**: `zonemta_mails_received_total`, `zonemta_mails_delivered_total`, `zonemta_mails_bounced_total`
- **Queue Management**: `zonemta_queue_size`, `zonemta_queue_age_seconds`, `zonemta_delivery_duration_seconds`
- **SMTP Operations**: `zonemta_smtp_connections`, `zonemta_smtp_commands_total`
- **System Health**: `zonemta_db_connections`, `zonemta_errors_total`
- **Rate Limiting**: `zonemta_rate_limit_hits_total`

#### Example Prometheus Queries

```promql
# Rate of emails received per minute
rate(zonemta_mails_received_total[1m])

# Current queue size by zone
zonemta_queue_size

# 95th percentile delivery time
histogram_quantile(0.95, zonemta_delivery_duration_seconds_bucket)

# Error rate per minute
rate(zonemta_errors_total[5m])
```

#### Grafana Dashboard

The metrics are designed to work with Grafana. Key visualizations:
- Email throughput over time
- Queue size trends
- Delivery performance histograms
- Error rate monitoring
- System resource usage

## Differences from Original ZoneMTA

While maintaining feature parity, ZoneMTA-Go has some differences:

- **Language**: Written in Go instead of Node.js
- **Configuration**: Uses YAML instead of JavaScript configuration
- **Performance**: Generally faster due to Go's performance characteristics
- **Memory Usage**: Lower memory footprint
- **Deployment**: Single binary deployment

## Development

### Building

```bash
go build -o zone-mta ./cmd/zone-mta
```

### Testing

```bash
go test ./...
```

### Project Structure

```
zone-mta-go/
├── cmd/zone-mta/          # Main application
├── internal/              # Internal packages
│   ├── api/              # HTTP API server
│   ├── config/           # Configuration management
│   ├── db/               # Database connections
│   ├── plugin/           # Plugin system
│   ├── queue/            # Mail queue management
│   ├── sender/           # Mail delivery
│   └── smtp/             # SMTP server
├── config/               # Configuration files
└── README.md
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

This project maintains the same license as the original ZoneMTA project.

## Support

For issues and questions:
- Create an issue on GitHub
- Check the original ZoneMTA documentation for concepts
- Review the configuration examples