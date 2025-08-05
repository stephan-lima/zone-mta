# SOCKS5 Proxy Plugin for ZoneMTA-Go

This plugin provides SOCKS5 proxy support for outgoing SMTP connections in ZoneMTA-Go. It allows you to route outgoing mail through SOCKS5 proxies on a per-connection basis with fine-grained rules.

## Features

- **Per-connection proxy routing**: Configure different proxies for different zones, recipients, or target hosts
- **Flexible rule system**: Match connections based on zone, recipient patterns, or target hosts
- **Authentication support**: Supports username/password authentication for SOCKS5 proxies
- **Outgoing IP information**: Provides information about the local IP that would be used for the connection
- **Fallback support**: Falls back to direct connections when no proxy rules match

## Configuration

Add the plugin to your ZoneMTA-Go configuration:

```yaml
plugins:
  socks5-proxy:
    enabled: true
    rules:
      # Route all mail from 'marketing' zone through proxy
      - zone: "marketing"
        proxyHost: "proxy1.example.com"
        proxyPort: 1080
        username: "user1"
        password: "pass1"
        enabled: true

      # Route mail to specific domain through different proxy
      - host: "gmail-smtp-in.l.google.com"
        proxyHost: "proxy2.example.com"
        proxyPort: 1080
        enabled: true

      # Route mail to specific recipient through proxy (with auth)
      - recipient: "user@example.com"
        proxyHost: "proxy3.example.com"
        proxyPort: 1080
        username: "proxyuser"
        password: "proxypass"
        enabled: true

      # Zone-specific routing with no authentication
      - zone: "bulk"
        proxyHost: "10.0.1.100"
        proxyPort: 1080
        enabled: true
```

## Rule Matching

Rules are evaluated in order, and the first matching rule is used. A rule matches when:

1. **Zone match**: If `zone` is specified, it must match the sending zone
2. **Recipient match**: If `recipient` is specified, it must match the recipient address
3. **Host match**: If `host` is specified, it must match the target SMTP server hostname

If multiple conditions are specified in a rule, ALL must match for the rule to apply.

### Rule Priority

Rules are processed in the order they appear in the configuration. The first rule that matches all specified conditions will be used.

## Configuration Options

### Rule Configuration

Each rule can contain the following fields:

- `zone` (string, optional): Match specific sending zone
- `recipient` (string, optional): Match specific recipient email address
- `host` (string, optional): Match specific target SMTP hostname
- `proxyHost` (string, required): SOCKS5 proxy hostname or IP
- `proxyPort` (int, required): SOCKS5 proxy port
- `username` (string, optional): SOCKS5 authentication username
- `password` (string, optional): SOCKS5 authentication password
- `enabled` (bool, required): Whether this rule is active

## Connection Information Available to Plugin

The plugin receives detailed information about each outgoing connection:

```go
type ConnectionData struct {
    TargetHost    string // Target MX hostname (e.g., "gmail-smtp-in.l.google.com")
    TargetPort    string // Target port (usually "25")
    LocalIP       string // Local outgoing IP address that would be used
    Zone          string // Sending zone name
    Recipient     string // Current recipient being delivered to
    MessageID     string // Message ID being delivered
    DeliveryCount int    // Number of delivery attempts
}
```

## Example Scenarios

### Scenario 1: Route Marketing Mail Through Proxy

```yaml
plugins:
  socks5-proxy:
    enabled: true
    rules:
      - zone: "marketing"
        proxyHost: "marketing-proxy.example.com"
        proxyPort: 1080
        enabled: true
```

All mail sent through the "marketing" zone will be routed through the specified proxy.

### Scenario 2: Route Specific Recipients Through Different Proxies

```yaml
plugins:
  socks5-proxy:
    enabled: true
    rules:
      - recipient: "newsletter@bigcorp.com"
        proxyHost: "proxy1.example.com"
        proxyPort: 1080
        enabled: true
      - recipient: "alerts@enterprise.com"
        proxyHost: "proxy2.example.com"
        proxyPort: 1080
        enabled: true
```

Different recipients are routed through different proxies for compliance or routing requirements.

### Scenario 3: Zone and Host-Specific Routing

```yaml
plugins:
  socks5-proxy:
    enabled: true
    rules:
      # Gmail traffic from bulk zone through specific proxy
      - zone: "bulk"
        host: "gmail-smtp-in.l.google.com"
        proxyHost: "gmail-proxy.example.com"
        proxyPort: 1080
        enabled: true
      
      # All other bulk traffic through different proxy
      - zone: "bulk"
        proxyHost: "bulk-proxy.example.com"
        proxyPort: 1080
        enabled: true
```

This configuration routes Gmail traffic from the bulk zone through one proxy, while other bulk traffic uses a different proxy.

## Logging

The plugin provides detailed logging at various levels:

- **Info**: When rules are applied and connections are established
- **Debug**: Detailed connection information and proxy details
- **Error**: When proxy connections fail

## Integration

To use this plugin in your ZoneMTA-Go application:

1. Import the plugin package
2. Register it with the plugin manager
3. Configure it in your YAML configuration

```go
import (
    socks5proxy "github.com/zone-eu/zone-mta-go/plugins/socks5-proxy"
    "github.com/zone-eu/zone-mta-go/internal/plugin"
)

// Register the plugin
pluginManager.RegisterPlugin(socks5proxy.NewSOCKS5ProxyPlugin(logger))
```

## Dependencies

- `golang.org/x/net/proxy` for SOCKS5 proxy support
- ZoneMTA-Go plugin system

## Security Considerations

- Store proxy credentials securely
- Use encrypted connections to proxy servers when possible
- Monitor proxy usage and connections
- Implement proper access controls for proxy servers

## Troubleshooting

Common issues:

1. **Proxy connection failures**: Check proxy host/port and network connectivity
2. **Authentication failures**: Verify username/password credentials
3. **Rules not matching**: Check rule order and matching criteria
4. **Performance issues**: Monitor proxy server performance and connection pooling

Enable debug logging to see detailed connection information:

```yaml
logging:
  level: debug
```