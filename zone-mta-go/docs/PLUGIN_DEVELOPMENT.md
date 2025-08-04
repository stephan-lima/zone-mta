# Plugin Development Guide for ZoneMTA-Go

This guide explains how to create custom plugins for ZoneMTA-Go. Plugins allow you to extend the functionality of the mail server by hooking into various stages of the email processing pipeline.

## Plugin Architecture

ZoneMTA-Go uses a hook-based plugin system where plugins can register functions to be called at specific points in the email processing lifecycle.

### Available Hook Points

- **`HookReceive`**: Called when a message is received via SMTP
- **`HookQueue`**: Called before a message is queued for delivery
- **`HookPreSend`**: Called before attempting to deliver a message
- **`HookPostSend`**: Called after a delivery attempt (success or failure)
- **`HookDelivered`**: Called when a message is successfully delivered
- **`HookBounce`**: Called when a message permanently bounces

## Plugin Interface

Every plugin must implement the `Plugin` interface:

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

## Creating a Simple Plugin

Here's a basic example plugin that logs message events:

```go
package myplugin

import (
    "context"
    "log/slog"
    
    "github.com/zone-eu/zone-mta-go/internal/plugin"
)

type MyPlugin struct {
    logger *slog.Logger
    config map[string]interface{}
}

func NewMyPlugin(logger *slog.Logger) *MyPlugin {
    return &MyPlugin{
        logger: logger.With("plugin", "my-plugin"),
    }
}

func (p *MyPlugin) Name() string {
    return "my-plugin"
}

func (p *MyPlugin) Description() string {
    return "My custom plugin description"
}

func (p *MyPlugin) Version() string {
    return "1.0.0"
}

func (p *MyPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
    p.config = config
    p.logger.Info("Plugin initialized", "config", config)
    return nil
}

func (p *MyPlugin) RegisterHooks(manager *plugin.Manager) error {
    manager.RegisterHook(plugin.HookReceive, p.onReceive)
    manager.RegisterHook(plugin.HookQueue, p.onQueue)
    return nil
}

func (p *MyPlugin) Shutdown(ctx context.Context) error {
    p.logger.Info("Plugin shutting down")
    return nil
}

func (p *MyPlugin) onReceive(ctx context.Context, data *plugin.HookData) error {
    if data.Message != nil {
        p.logger.Info("Message received", 
            "messageId", data.Message.MessageID,
            "from", data.Message.From)
    }
    return nil
}

func (p *MyPlugin) onQueue(ctx context.Context, data *plugin.HookData) error {
    if data.Message != nil {
        p.logger.Info("Message queued", 
            "messageId", data.Message.MessageID,
            "zone", data.Message.Zone)
    }
    return nil
}
```

## Hook Data Structure

The `HookData` structure contains information about the current message and context:

```go
type HookData struct {
    Message    *queue.QueuedMessage   // Current message
    Content    []byte                 // Message content (when available)
    Error      error                  // Any error that occurred
    Metadata   map[string]interface{} // Additional metadata
    Interface  string                 // SMTP interface name
    RemoteIP   string                 // Client IP address
    Result     *HookResult            // Hook execution result
}
```

## Hook Results

Plugins can influence message processing by setting the `Result` field:

```go
type HookResult struct {
    Accept   bool                   // Accept the message (stop further processing)
    Reject   bool                   // Reject the message
    Message  string                 // Custom rejection/response message
    Headers  map[string]string      // Additional headers to add
    Metadata map[string]interface{} // Additional metadata
}
```

### Example: Rejecting Messages

```go
func (p *MyPlugin) onReceive(ctx context.Context, data *plugin.HookData) error {
    if data.Message != nil && strings.Contains(data.Message.From, "spam") {
        if data.Result == nil {
            data.Result = &plugin.HookResult{}
        }
        data.Result.Reject = true
        data.Result.Message = "Sender not allowed"
    }
    return nil
}
```

## Configuration

Plugin configuration is defined in the main configuration file:

```yaml
plugins:
  my-plugin:
    enabled: true
    someOption: "value"
    numericOption: 42
    listOption:
      - "item1"
      - "item2"
```

Access configuration in your plugin:

```go
func (p *MyPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
    // Parse configuration
    if enabled, ok := config["enabled"].(bool); ok && !enabled {
        return nil // Plugin disabled
    }
    
    if option, ok := config["someOption"].(string); ok {
        p.someOption = option
    }
    
    return nil
}
```

## Advanced Plugin: Rate Limiter

Here's a more complex example that demonstrates database integration:

```go
package ratelimiter

import (
    "context"
    "fmt"
    "log/slog"
    "strconv"
    "strings"
    "time"
    
    "github.com/zone-eu/zone-mta-go/internal/db"
    "github.com/zone-eu/zone-mta-go/internal/plugin"
)

type RateLimiterPlugin struct {
    redis  *db.Redis
    config *Config
    logger *slog.Logger
}

type Config struct {
    Enabled bool              `yaml:"enabled"`
    Limits  map[string]string `yaml:"limits"`
}

func NewRateLimiterPlugin(redis *db.Redis, logger *slog.Logger) *RateLimiterPlugin {
    return &RateLimiterPlugin{
        redis:  redis,
        logger: logger.With("plugin", "rate-limiter"),
    }
}

func (p *RateLimiterPlugin) checkRateLimit(ctx context.Context, data *plugin.HookData) error {
    // Check if rate limit is exceeded
    key := fmt.Sprintf("rate:%s:%s", data.Message.From, time.Now().Format("2006010215"))
    
    count, err := p.redis.IncrementWithExpire(ctx, key, time.Hour)
    if err != nil {
        return err
    }
    
    if count > 100 { // Limit: 100 emails per hour
        if data.Result == nil {
            data.Result = &plugin.HookResult{}
        }
        data.Result.Reject = true
        data.Result.Message = "Rate limit exceeded"
    }
    
    return nil
}
```

## Plugin Registration

### Built-in Plugins

Add your plugin to the built-in registry:

```go
// In internal/plugin/registry.go
var BuiltinPlugins = map[string]BuiltinPluginFactory{
    "my-plugin": func(logger *slog.Logger) Plugin {
        return myplugin.NewMyPlugin(logger)
    },
}
```

### External Plugins with Dependencies

For plugins that need database access:

```go
// In internal/plugin/registry.go
var ExternalPlugins = map[string]ExternalPluginFactory{
    "rate-limiter": func(dbMgr *db.Manager, logger *slog.Logger) Plugin {
        return ratelimiter.NewRateLimiterPlugin(dbMgr.Redis, logger)
    },
}
```

## Testing Plugins

Create unit tests for your plugins:

```go
package myplugin_test

import (
    "context"
    "testing"
    "log/slog"
    "os"
    
    "github.com/zone-eu/zone-mta-go/internal/plugin"
    "github.com/zone-eu/zone-mta-go/internal/queue"
)

func TestMyPlugin(t *testing.T) {
    logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
    p := NewMyPlugin(logger)
    
    // Test initialization
    config := map[string]interface{}{
        "enabled": true,
        "someOption": "test",
    }
    
    err := p.Initialize(context.Background(), config)
    if err != nil {
        t.Fatalf("Failed to initialize plugin: %v", err)
    }
    
    // Test hook execution
    data := &plugin.HookData{
        Message: &queue.QueuedMessage{
            MessageID: "test-123",
            From:      "test@example.com",
        },
    }
    
    err = p.onReceive(context.Background(), data)
    if err != nil {
        t.Fatalf("Hook execution failed: %v", err)
    }
}
```

## Best Practices

1. **Error Handling**: Always handle errors gracefully. Plugin failures shouldn't crash the server.

2. **Performance**: Keep hook functions fast. Long-running operations can block message processing.

3. **Logging**: Use structured logging with appropriate levels.

4. **Configuration**: Validate configuration during initialization.

5. **Thread Safety**: Ensure your plugin is safe for concurrent use.

6. **Resource Cleanup**: Clean up resources in the `Shutdown` method.

## Example Plugins Included

The ZoneMTA-Go codebase includes several example plugins:

1. **Default Headers Plugin** (`internal/plugin/manager.go`): Adds standard email headers
2. **Message Logger Plugin** (`plugins/message-logger/`): Logs message events for debugging
3. **Rate Limiter Plugin** (`plugins/rate-limiter/`): Implements rate limiting based on sender and IP

## Plugin Ideas

Here are some ideas for useful plugins:

- **DKIM Signing**: Sign outgoing messages with DKIM
- **SPF Validation**: Validate incoming messages against SPF records
- **Content Filtering**: Block messages containing specific content
- **Virus Scanning**: Integration with antivirus engines
- **Backup**: Store copies of messages in external storage
- **Metrics**: Custom metrics collection
- **Webhooks**: Send notifications to external services
- **IP Reputation**: Check sender IP against reputation services

## Debugging Plugins

Enable debug logging to see plugin execution:

```yaml
logging:
  level: "debug"
```

Use the message logger plugin to trace message flow:

```yaml
plugins:
  message-logger:
    enabled: true
    logLevel: "debug"
    logEvents: ["receive", "queue", "pre_send", "post_send", "delivered", "bounced"]
```

## Contributing Plugins

To contribute a plugin to the ZoneMTA-Go project:

1. Create the plugin in the `plugins/` directory
2. Add comprehensive tests
3. Update documentation
4. Add configuration examples
5. Submit a pull request

For questions or help with plugin development, please open an issue on the project repository.