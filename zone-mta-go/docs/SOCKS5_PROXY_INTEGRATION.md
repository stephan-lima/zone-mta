# SOCKS5 Proxy Integration for ZoneMTA-Go

This document describes the implementation of SOCKS5 proxy support for outgoing SMTP connections in ZoneMTA-Go through the plugin system.

## Overview

The SOCKS5 proxy integration allows you to route outgoing SMTP connections through SOCKS5 proxies on a per-connection basis. This provides fine-grained control over how different types of mail are delivered, including:

- Zone-based routing
- Recipient-based routing  
- Target host-based routing
- IP address information for connection decisions

## Architecture

### Plugin Hook System

The implementation adds a new hook type `HookConnection` to the plugin system that is called before establishing outgoing SMTP connections.

```go
const (
    // ... existing hooks
    HookConnection HookType = "connection" // Before establishing outgoing SMTP connection
)
```

### Hook Data Structure

The hook receives detailed connection information:

```go
type ConnectionData struct {
    TargetHost    string // Target MX hostname
    TargetPort    string // Target port (usually "25")
    LocalIP       string // Local outgoing IP address
    Zone          string // Sending zone name
    Recipient     string // Current recipient being delivered to
    MessageID     string // Message ID being delivered
    DeliveryCount int    // Number of delivery attempts
}
```

### Hook Result

Plugins can provide a custom connection through the hook result:

```go
type HookResult struct {
    // ... existing fields
    Connection net.Conn `json:"-"` // Custom connection provided by plugin
}
```

## Integration Points

### Sender Integration

The `sender.go` has been modified to:

1. Accept a plugin manager in the constructor
2. Call connection hooks before establishing SMTP connections
3. Use plugin-provided connections when available
4. Fall back to default connections when no plugin provides one

Key changes:
- `NewSender()` now accepts a `*plugin.Manager` parameter
- `deliverViaMX()` uses `connectWithHooks()` instead of direct `net.DialTimeout()`
- `connectWithHooks()` determines the local IP and calls plugin hooks

### Plugin Manager Extensions

The plugin manager now supports:
- `HookConnection` hook type
- Extended `HookData` with connection information
- Extended `HookResult` with custom connection support

## Usage Example

### Configuration

```yaml
plugins:
  socks5-proxy:
    enabled: true
    rules:
      - zone: "marketing"
        proxyHost: "proxy1.example.com"
        proxyPort: 1080
        username: "user1"
        password: "pass1"
        enabled: true
      
      - host: "gmail-smtp-in.l.google.com"
        proxyHost: "proxy2.example.com"
        proxyPort: 1080
        enabled: true
```

### Plugin Registration

```go
// Initialize plugin manager
pluginManager := plugin.NewManager(cfg.Plugins, logger)

// Register SOCKS5 proxy plugin
socks5Plugin := socks5proxy.NewSOCKS5ProxyPlugin(logger)
pluginManager.RegisterPlugin(socks5Plugin)

// Initialize sender with plugin support
senderManager := sender.NewSenderManager(queueManager, pluginManager, logger)
```

## Rule Matching System

The SOCKS5 proxy plugin supports flexible rule matching:

### Rule Types

1. **Zone-based**: Match by sending zone
   ```yaml
   - zone: "marketing"
     proxyHost: "proxy.example.com"
     proxyPort: 1080
   ```

2. **Recipient-based**: Match by recipient email
   ```yaml
   - recipient: "user@example.com"
     proxyHost: "proxy.example.com"
     proxyPort: 1080
   ```

3. **Host-based**: Match by target SMTP server
   ```yaml
   - host: "gmail-smtp-in.l.google.com"
     proxyHost: "proxy.example.com"
     proxyPort: 1080
   ```

4. **Combined rules**: Multiple conditions must all match
   ```yaml
   - zone: "bulk"
     host: "gmail-smtp-in.l.google.com"
     proxyHost: "gmail-proxy.example.com"
     proxyPort: 1080
   ```

### Rule Priority

Rules are processed in the order they appear in the configuration. The first rule that matches all specified conditions is used.

## Connection Flow

1. **Message Processing**: Worker retrieves message from queue
2. **MX Lookup**: Resolve MX records for recipient domain
3. **Connection Hook**: 
   - Call `connectWithHooks()` with connection details
   - Execute `HookConnection` hooks with connection data
   - Plugin evaluates rules and may provide custom connection
4. **Connection Establishment**:
   - If plugin provides connection: use it
   - Otherwise: fall back to default `net.DialTimeout()`
5. **SMTP Delivery**: Proceed with normal SMTP protocol

## Advanced Features

### Local IP Detection

The system determines the local IP that would be used for the connection by creating a test connection. This information is available to plugins for routing decisions.

### Authentication Support

The SOCKS5 plugin supports username/password authentication:

```yaml
- zone: "secure"
  proxyHost: "auth-proxy.example.com"
  proxyPort: 1080
  username: "myuser"
  password: "mypass"
  enabled: true
```

### Flexible Configuration

Rules support optional fields for maximum flexibility:
- `zone`: Match specific sending zone (optional)
- `recipient`: Match specific recipient (optional)  
- `host`: Match specific target host (optional)
- `proxyHost`: SOCKS5 proxy hostname (required)
- `proxyPort`: SOCKS5 proxy port (required)
- `username`: Authentication username (optional)
- `password`: Authentication password (optional)
- `enabled`: Enable/disable rule (required)

## Performance Considerations

### Connection Pooling

The implementation works with ZoneMTA-Go's existing connection pooling system. Proxy connections are treated as regular connections for pooling purposes.

### Local IP Detection

The local IP detection creates a brief test connection. This adds minimal overhead but provides valuable routing information.

### Plugin Performance

Plugin hooks are called synchronously in the delivery path. Plugins should:
- Make routing decisions quickly
- Cache proxy connections when possible
- Handle errors gracefully to avoid blocking delivery

## Security Considerations

### Credential Management

- Store proxy credentials securely in configuration
- Use environment variables or secure vaults for sensitive data
- Rotate credentials regularly

### Network Security

- Use authenticated proxies when possible
- Monitor proxy usage and connections
- Implement proper access controls for proxy servers
- Consider using encrypted proxy protocols

### Error Handling

- Plugins should handle proxy failures gracefully
- Failed proxy connections fall back to direct connections
- Log proxy failures for monitoring

## Monitoring and Debugging

### Logging

Enable debug logging to see detailed connection information:

```yaml
logging:
  level: debug
```

This will show:
- When hooks are executed
- Connection details (target host, local IP, zone, etc.)
- Proxy routing decisions
- Connection establishment details

### Metrics

The existing metrics system can be extended to track:
- Proxy vs direct connections
- Proxy success/failure rates
- Connection performance by proxy

## Extension Points

The plugin system is designed to be extensible:

### Custom Proxy Types

Create plugins for other proxy types (HTTP CONNECT, custom protocols):

```go
type MyProxyPlugin struct {
    // Plugin implementation
}

func (p *MyProxyPlugin) RegisterHooks(manager *plugin.Manager) error {
    manager.RegisterHook(plugin.HookConnection, p.handleConnection)
    return nil
}
```

### Advanced Routing

Implement complex routing logic:
- Load balancing across multiple proxies
- Failover between proxy servers
- Geographic routing based on target IP
- Time-based routing rules

### Integration with External Systems

- DNS-based proxy selection
- Database-driven routing rules
- API-based proxy configuration
- Real-time proxy health monitoring

## Migration and Compatibility

### Backward Compatibility

The implementation maintains full backward compatibility:
- Existing configurations continue to work
- No changes to core SMTP delivery logic
- Optional plugin system - can be disabled

### Migration Path

1. Update ZoneMTA-Go to version with plugin support
2. Configure SOCKS5 proxy plugin
3. Test with subset of traffic
4. Gradually migrate traffic to proxy routing
5. Monitor and optimize configuration

## Troubleshooting

### Common Issues

1. **Plugin not loaded**: Check plugin configuration and registration
2. **Rules not matching**: Verify rule order and matching criteria  
3. **Proxy connection failures**: Check proxy server and network connectivity
4. **Authentication failures**: Verify proxy credentials
5. **Performance issues**: Monitor proxy server performance

### Debug Steps

1. Enable debug logging
2. Check plugin registration in logs
3. Verify rule matching with test messages
4. Monitor proxy server logs
5. Test proxy connectivity independently