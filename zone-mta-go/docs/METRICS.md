# Metrics and Monitoring Guide for ZoneMTA-Go

ZoneMTA-Go includes comprehensive Prometheus metrics for monitoring email processing, system health, and performance.

## Metrics Server

The metrics server runs independently from the main application on a configurable port (default: 9090).

### Configuration

```yaml
# config/default.yaml
metrics:
  enabled: true
  port: 9090
  path: "/metrics"
```

### Endpoints

- **Metrics**: `http://localhost:9090/metrics` - Prometheus metrics
- **Health**: `http://localhost:9090/health` - Health check endpoint

## Available Metrics

### Mail Processing Metrics

#### `zonemta_mails_received_total`
- **Type**: Counter
- **Description**: Total number of emails received
- **Labels**: `interface` (smtp, api), `zone`

#### `zonemta_mails_queued_total`
- **Type**: Counter
- **Description**: Total number of emails queued for delivery
- **Labels**: `zone`

#### `zonemta_mails_delivered_total`
- **Type**: Counter
- **Description**: Total number of emails successfully delivered
- **Labels**: `zone`

#### `zonemta_mails_bounced_total`
- **Type**: Counter
- **Description**: Total number of emails that bounced
- **Labels**: `zone`, `reason`

#### `zonemta_mails_deferred_total`
- **Type**: Counter
- **Description**: Total number of emails deferred for retry
- **Labels**: `zone`

### Queue Metrics

#### `zonemta_queue_size`
- **Type**: Gauge
- **Description**: Current number of emails in queue
- **Labels**: `zone`, `status` (queued, sending, deferred)

#### `zonemta_queue_age_seconds`
- **Type**: Histogram
- **Description**: Age of emails in queue
- **Labels**: `zone`
- **Buckets**: Exponential from 1s to ~9h

#### `zonemta_delivery_duration_seconds`
- **Type**: Histogram
- **Description**: Time taken to deliver emails
- **Labels**: `zone`
- **Buckets**: Exponential from 100ms to ~2min

### SMTP Metrics

#### `zonemta_smtp_connections`
- **Type**: Gauge
- **Description**: Current number of SMTP connections
- **Labels**: `interface`, `type` (inbound, outbound)

#### `zonemta_smtp_commands_total`
- **Type**: Counter
- **Description**: Total number of SMTP commands processed
- **Labels**: `interface`, `command`, `status`

### System Metrics

#### `zonemta_db_connections`
- **Type**: Gauge
- **Description**: Current number of database connections
- **Labels**: `database` (mongodb, redis)

#### `zonemta_processing_duration_seconds`
- **Type**: Histogram
- **Description**: Time taken for various processing operations
- **Labels**: `operation`

#### `zonemta_errors_total`
- **Type**: Counter
- **Description**: Total number of errors by type
- **Labels**: `component`, `type`

### Rate Limiting Metrics

#### `zonemta_rate_limit_hits_total`
- **Type**: Counter
- **Description**: Total number of rate limit hits
- **Labels**: `type` (sender, ip, interface), `action` (defer, reject)

## Prometheus Configuration

### Basic Prometheus Config

```yaml
# prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'zonemta'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 10s
    metrics_path: /metrics
```

### Docker Compose with Prometheus

```yaml
version: '3.8'
services:
  zone-mta-go:
    image: zone-mta-go
    ports:
      - "2525:2525"
      - "12080:12080"
      - "9090:9090"

  prometheus:
    image: prom/prometheus
    ports:
      - "9091:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'

  grafana:
    image: grafana/grafana
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
```

## Example Queries

### Email Throughput

```promql
# Emails received per minute
rate(zonemta_mails_received_total[1m])

# Emails delivered per minute by zone
rate(zonemta_mails_delivered_total[1m]) by (zone)

# Bounce rate percentage
rate(zonemta_mails_bounced_total[1m]) / rate(zonemta_mails_received_total[1m]) * 100
```

### Queue Monitoring

```promql
# Current queue size
zonemta_queue_size

# Queue size by zone and status
zonemta_queue_size by (zone, status)

# Average queue age
rate(zonemta_queue_age_seconds_sum[5m]) / rate(zonemta_queue_age_seconds_count[5m])
```

### Performance Metrics

```promql
# 95th percentile delivery time
histogram_quantile(0.95, zonemta_delivery_duration_seconds_bucket)

# Average delivery time by zone
rate(zonemta_delivery_duration_seconds_sum[5m]) / rate(zonemta_delivery_duration_seconds_count[5m]) by (zone)

# Slow deliveries (>10 seconds)
increase(zonemta_delivery_duration_seconds_bucket{le="10"}[5m])
```

### Error Monitoring

```promql
# Error rate per minute
rate(zonemta_errors_total[1m])

# Errors by component
rate(zonemta_errors_total[5m]) by (component)

# Rate limit hits
rate(zonemta_rate_limit_hits_total[1m]) by (type, action)
```

### System Health

```promql
# Database connections
zonemta_db_connections

# SMTP connections by type
zonemta_smtp_connections by (interface, type)

# Processing time by operation
histogram_quantile(0.95, zonemta_processing_duration_seconds_bucket) by (operation)
```

## Grafana Dashboard

### Key Panels

1. **Email Overview**
   - Total emails received (single stat)
   - Delivery rate (graph)
   - Bounce rate (graph)
   - Queue size (graph)

2. **Performance**
   - Delivery time histogram
   - Queue age distribution
   - Processing time by operation

3. **System Health**
   - Database connections
   - SMTP connections
   - Error rate
   - Memory/CPU usage (from node_exporter)

4. **Rate Limiting**
   - Rate limit hits by type
   - Blocked/deferred messages

### Sample Dashboard JSON

```json
{
  "dashboard": {
    "title": "ZoneMTA-Go Dashboard",
    "panels": [
      {
        "title": "Emails Received",
        "type": "singlestat",
        "targets": [
          {
            "expr": "sum(rate(zonemta_mails_received_total[1m]))",
            "legendFormat": "per minute"
          }
        ]
      },
      {
        "title": "Queue Size",
        "type": "graph",
        "targets": [
          {
            "expr": "zonemta_queue_size by (zone)",
            "legendFormat": "{{zone}}"
          }
        ]
      }
    ]
  }
}
```

## Alerting Rules

### Prometheus Alerting

```yaml
# alerts.yml
groups:
  - name: zonemta
    rules:
      - alert: HighBounceRate
        expr: rate(zonemta_mails_bounced_total[5m]) / rate(zonemta_mails_received_total[5m]) > 0.1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High bounce rate detected"
          description: "Bounce rate is {{ $value }}% for the last 5 minutes"

      - alert: QueueBacklog
        expr: zonemta_queue_size > 1000
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Large queue backlog"
          description: "Queue size is {{ $value }} messages"

      - alert: SlowDeliveries
        expr: histogram_quantile(0.95, zonemta_delivery_duration_seconds_bucket) > 30
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Slow email deliveries"
          description: "95th percentile delivery time is {{ $value }} seconds"

      - alert: HighErrorRate
        expr: rate(zonemta_errors_total[5m]) > 10
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "High error rate"
          description: "Error rate is {{ $value }} errors per minute"
```

## Testing Metrics

### Manual Testing

```bash
# Start the application
./zone-mta -config config/default.yaml

# Check metrics endpoint
curl http://localhost:9090/metrics

# Send test emails and monitor metrics
curl -X POST http://localhost:12080/api/v1/queue/send \
  -H "Content-Type: application/json" \
  -d '{"from":"test@example.com","to":["user@example.com"],"message":"..."}'

# Check updated metrics
curl http://localhost:9090/metrics | grep zonemta_mails_received_total
```

### Automated Testing

```bash
# Run metrics example
make metrics-example

# In another terminal, verify metrics
curl http://localhost:9090/metrics | grep zonemta_
```

### Load Testing

```bash
# Generate load and monitor metrics
for i in {1..100}; do
  curl -X POST http://localhost:12080/api/v1/queue/send \
    -H "Content-Type: application/json" \
    -d "{\"from\":\"test$i@example.com\",\"to\":[\"user$i@example.com\"],\"message\":\"Test $i\"}"
done

# Monitor queue metrics
watch "curl -s http://localhost:9090/metrics | grep zonemta_queue_size"
```

## Integration Examples

### With Kubernetes

```yaml
# ServiceMonitor for Prometheus Operator
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: zonemta
spec:
  selector:
    matchLabels:
      app: zonemta
  endpoints:
  - port: metrics
    path: /metrics
    interval: 30s
```

### With Docker Swarm

```yaml
# docker-compose.yml with Prometheus
version: '3.8'
services:
  zonemta:
    image: zone-mta-go
    deploy:
      labels:
        - "prometheus.enable=true"
        - "prometheus.port=9090"
        - "prometheus.path=/metrics"
```

This comprehensive metrics system provides full observability into ZoneMTA-Go operations, enabling effective monitoring, alerting, and performance optimization.