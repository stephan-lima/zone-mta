package messagelogger

import (
	"context"
	"log/slog"
	"strings"

	"github.com/zone-eu/zone-mta-go/internal/plugin"
)

// MessageLoggerPlugin logs message events for debugging and monitoring
type MessageLoggerPlugin struct {
	config *Config
	logger *slog.Logger
}

// Config represents the message logger configuration
type Config struct {
	Enabled       bool     `yaml:"enabled"`
	LogLevel      string   `yaml:"logLevel"`      // debug, info, warn, error
	LogEvents     []string `yaml:"logEvents"`     // which events to log
	IncludeBody   bool     `yaml:"includeBody"`   // whether to log message body
	MaxBodyLength int      `yaml:"maxBodyLength"` // max body length to log
}

// NewMessageLoggerPlugin creates a new message logger plugin
func NewMessageLoggerPlugin(logger *slog.Logger) *MessageLoggerPlugin {
	return &MessageLoggerPlugin{
		logger: logger.With("plugin", "message-logger"),
	}
}

// Name returns the plugin name
func (p *MessageLoggerPlugin) Name() string {
	return "message-logger"
}

// Description returns the plugin description
func (p *MessageLoggerPlugin) Description() string {
	return "Logs message events for debugging and monitoring purposes"
}

// Version returns the plugin version
func (p *MessageLoggerPlugin) Version() string {
	return "1.0.0"
}

// Initialize initializes the plugin with configuration
func (p *MessageLoggerPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	// Set default configuration
	p.config = &Config{
		Enabled:       true,
		LogLevel:      "info",
		LogEvents:     []string{"receive", "queue", "delivered", "bounced"},
		IncludeBody:   false,
		MaxBodyLength: 1000,
	}

	// Parse configuration
	if enabled, ok := config["enabled"].(bool); ok {
		p.config.Enabled = enabled
	}

	if logLevel, ok := config["logLevel"].(string); ok {
		p.config.LogLevel = logLevel
	}

	if events, ok := config["logEvents"].([]interface{}); ok {
		p.config.LogEvents = make([]string, len(events))
		for i, event := range events {
			if eventStr, ok := event.(string); ok {
				p.config.LogEvents[i] = eventStr
			}
		}
	}

	if includeBody, ok := config["includeBody"].(bool); ok {
		p.config.IncludeBody = includeBody
	}

	if maxBodyLength, ok := config["maxBodyLength"].(int); ok {
		p.config.MaxBodyLength = maxBodyLength
	}

	p.logger.Info("Message logger plugin initialized",
		"enabled", p.config.Enabled,
		"logLevel", p.config.LogLevel,
		"events", p.config.LogEvents)

	return nil
}

// RegisterHooks registers the plugin hooks
func (p *MessageLoggerPlugin) RegisterHooks(manager *plugin.Manager) error {
	if !p.config.Enabled {
		return nil
	}

	// Register hooks for all events we want to log
	for _, event := range p.config.LogEvents {
		switch event {
		case "receive":
			manager.RegisterHook(plugin.HookReceive, p.logReceive)
		case "queue":
			manager.RegisterHook(plugin.HookQueue, p.logQueue)
		case "pre_send":
			manager.RegisterHook(plugin.HookPreSend, p.logPreSend)
		case "post_send":
			manager.RegisterHook(plugin.HookPostSend, p.logPostSend)
		case "delivered":
			manager.RegisterHook(plugin.HookDelivered, p.logDelivered)
		case "bounced":
			manager.RegisterHook(plugin.HookBounce, p.logBounced)
		}
	}

	p.logger.Info("Message logger hooks registered")
	return nil
}

// Shutdown cleans up the plugin
func (p *MessageLoggerPlugin) Shutdown(ctx context.Context) error {
	p.logger.Info("Message logger plugin shutting down")
	return nil
}

// logReceive logs when a message is received
func (p *MessageLoggerPlugin) logReceive(ctx context.Context, data *plugin.HookData) error {
	if data.Message == nil {
		return nil
	}

	logData := []interface{}{
		"event", "receive",
		"messageId", data.Message.MessageID,
		"from", data.Message.From,
		"recipients", len(data.Message.Recipients),
		"interface", data.Interface,
		"remoteIp", data.RemoteIP,
	}

	if p.config.IncludeBody && data.Content != nil {
		content := string(data.Content)
		if len(content) > p.config.MaxBodyLength {
			content = content[:p.config.MaxBodyLength] + "..."
		}
		logData = append(logData, "content", content)
	}

	p.logWithLevel("Message received", logData...)
	return nil
}

// logQueue logs when a message is queued
func (p *MessageLoggerPlugin) logQueue(ctx context.Context, data *plugin.HookData) error {
	if data.Message == nil {
		return nil
	}

	logData := []interface{}{
		"event", "queue",
		"messageId", data.Message.MessageID,
		"from", data.Message.From,
		"recipients", data.Message.Recipients,
		"zone", data.Message.Zone,
		"size", data.Message.MessageSize,
	}

	if data.Message.Subject != "" {
		logData = append(logData, "subject", data.Message.Subject)
	}

	p.logWithLevel("Message queued", logData...)
	return nil
}

// logPreSend logs before a message is sent
func (p *MessageLoggerPlugin) logPreSend(ctx context.Context, data *plugin.HookData) error {
	if data.Message == nil {
		return nil
	}

	p.logWithLevel("Message sending started",
		"event", "pre_send",
		"messageId", data.Message.MessageID,
		"from", data.Message.From,
		"recipients", data.Message.Recipients,
		"attempts", data.Message.DeliveryAttempts)

	return nil
}

// logPostSend logs after a message send attempt
func (p *MessageLoggerPlugin) logPostSend(ctx context.Context, data *plugin.HookData) error {
	if data.Message == nil {
		return nil
	}

	logData := []interface{}{
		"event", "post_send",
		"messageId", data.Message.MessageID,
		"from", data.Message.From,
		"attempts", data.Message.DeliveryAttempts,
	}

	if data.Error != nil {
		logData = append(logData, "error", data.Error.Error())
	}

	p.logWithLevel("Message send completed", logData...)
	return nil
}

// logDelivered logs when a message is successfully delivered
func (p *MessageLoggerPlugin) logDelivered(ctx context.Context, data *plugin.HookData) error {
	if data.Message == nil {
		return nil
	}

	p.logWithLevel("Message delivered successfully",
		"event", "delivered",
		"messageId", data.Message.MessageID,
		"from", data.Message.From,
		"recipients", data.Message.Recipients,
		"attempts", data.Message.DeliveryAttempts,
		"queueTime", data.Message.QueueTime,
		"deliveryTime", data.Message.DeliveryTime)

	return nil
}

// logBounced logs when a message bounces
func (p *MessageLoggerPlugin) logBounced(ctx context.Context, data *plugin.HookData) error {
	if data.Message == nil {
		return nil
	}

	logData := []interface{}{
		"event", "bounced",
		"messageId", data.Message.MessageID,
		"from", data.Message.From,
		"recipients", data.Message.Recipients,
		"attempts", data.Message.DeliveryAttempts,
	}

	if data.Message.BounceReason != "" {
		logData = append(logData, "bounceReason", data.Message.BounceReason)
	}

	if data.Error != nil {
		logData = append(logData, "error", data.Error.Error())
	}

	p.logWithLevel("Message bounced", logData...)
	return nil
}

// logWithLevel logs a message with the configured log level
func (p *MessageLoggerPlugin) logWithLevel(message string, args ...interface{}) {
	switch strings.ToLower(p.config.LogLevel) {
	case "debug":
		p.logger.Debug(message, args...)
	case "info":
		p.logger.Info(message, args...)
	case "warn":
		p.logger.Warn(message, args...)
	case "error":
		p.logger.Error(message, args...)
	default:
		p.logger.Info(message, args...)
	}
}
