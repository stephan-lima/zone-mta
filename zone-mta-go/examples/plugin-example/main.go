package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/zone-eu/zone-mta-go/internal/plugin"
	"github.com/zone-eu/zone-mta-go/internal/queue"
)

// SimpleExamplePlugin demonstrates a basic plugin implementation
type SimpleExamplePlugin struct {
	logger *slog.Logger
	config map[string]interface{}
}

// NewSimpleExamplePlugin creates a new simple example plugin
func NewSimpleExamplePlugin(logger *slog.Logger) *SimpleExamplePlugin {
	return &SimpleExamplePlugin{
		logger: logger.With("plugin", "simple-example"),
	}
}

// Name returns the plugin name
func (p *SimpleExamplePlugin) Name() string {
	return "simple-example"
}

// Description returns the plugin description
func (p *SimpleExamplePlugin) Description() string {
	return "A simple example plugin that demonstrates basic functionality"
}

// Version returns the plugin version
func (p *SimpleExamplePlugin) Version() string {
	return "1.0.0"
}

// Initialize initializes the plugin with configuration
func (p *SimpleExamplePlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.config = config

	// Log initialization with configuration
	p.logger.Info("Simple example plugin initialized", "config", config)

	return nil
}

// RegisterHooks registers the plugin hooks
func (p *SimpleExamplePlugin) RegisterHooks(manager *plugin.Manager) error {
	// Register hooks for different stages
	manager.RegisterHook(plugin.HookReceive, p.onMessageReceived)
	manager.RegisterHook(plugin.HookQueue, p.onMessageQueued)
	manager.RegisterHook(plugin.HookDelivered, p.onMessageDelivered)
	manager.RegisterHook(plugin.HookBounce, p.onMessageBounced)

	p.logger.Info("Plugin hooks registered")
	return nil
}

// Shutdown cleans up the plugin
func (p *SimpleExamplePlugin) Shutdown(ctx context.Context) error {
	p.logger.Info("Simple example plugin shutting down")
	return nil
}

// onMessageReceived is called when a message is received
func (p *SimpleExamplePlugin) onMessageReceived(ctx context.Context, data *plugin.HookData) error {
	if data.Message == nil {
		return nil
	}

	p.logger.Info("📨 Message received",
		"messageId", data.Message.MessageID,
		"from", data.Message.From,
		"recipients", len(data.Message.Recipients),
		"interface", data.Interface,
		"remoteIp", data.RemoteIP)

	// Example: Add custom metadata
	if data.Metadata == nil {
		data.Metadata = make(map[string]interface{})
	}
	data.Metadata["receivedAt"] = data.Message.Created
	data.Metadata["plugin"] = "simple-example"

	return nil
}

// onMessageQueued is called when a message is queued for delivery
func (p *SimpleExamplePlugin) onMessageQueued(ctx context.Context, data *plugin.HookData) error {
	if data.Message == nil {
		return nil
	}

	p.logger.Info("📥 Message queued",
		"messageId", data.Message.MessageID,
		"from", data.Message.From,
		"zone", data.Message.Zone,
		"size", data.Message.MessageSize)

	// Example: Add custom headers
	if data.Result == nil {
		data.Result = &plugin.HookResult{}
	}
	if data.Result.Headers == nil {
		data.Result.Headers = make(map[string]string)
	}
	data.Result.Headers["X-Processed-By"] = "SimpleExamplePlugin"

	return nil
}

// onMessageDelivered is called when a message is successfully delivered
func (p *SimpleExamplePlugin) onMessageDelivered(ctx context.Context, data *plugin.HookData) error {
	if data.Message == nil {
		return nil
	}

	p.logger.Info("✅ Message delivered",
		"messageId", data.Message.MessageID,
		"from", data.Message.From,
		"attempts", data.Message.DeliveryAttempts,
		"queueTime", data.Message.QueueTime)

	return nil
}

// onMessageBounced is called when a message bounces
func (p *SimpleExamplePlugin) onMessageBounced(ctx context.Context, data *plugin.HookData) error {
	if data.Message == nil {
		return nil
	}

	p.logger.Warn("❌ Message bounced",
		"messageId", data.Message.MessageID,
		"from", data.Message.From,
		"attempts", data.Message.DeliveryAttempts,
		"bounceReason", data.Message.BounceReason)

	return nil
}

// DemoFilterPlugin demonstrates message filtering capabilities
type DemoFilterPlugin struct {
	logger *slog.Logger
	config map[string]interface{}
}

// NewDemoFilterPlugin creates a new demo filter plugin
func NewDemoFilterPlugin(logger *slog.Logger) *DemoFilterPlugin {
	return &DemoFilterPlugin{
		logger: logger.With("plugin", "demo-filter"),
	}
}

func (p *DemoFilterPlugin) Name() string        { return "demo-filter" }
func (p *DemoFilterPlugin) Description() string { return "Demo plugin showing message filtering" }
func (p *DemoFilterPlugin) Version() string     { return "1.0.0" }

func (p *DemoFilterPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.config = config
	p.logger.Info("Demo filter plugin initialized")
	return nil
}

func (p *DemoFilterPlugin) RegisterHooks(manager *plugin.Manager) error {
	manager.RegisterHook(plugin.HookReceive, p.filterMessage)
	return nil
}

func (p *DemoFilterPlugin) Shutdown(ctx context.Context) error {
	p.logger.Info("Demo filter plugin shutting down")
	return nil
}

// filterMessage demonstrates message filtering
func (p *DemoFilterPlugin) filterMessage(ctx context.Context, data *plugin.HookData) error {
	if data.Message == nil {
		return nil
	}

	// Example: Block messages from specific domains
	blockedDomains := []string{"spam.example.com", "blocked.test"}

	for _, domain := range blockedDomains {
		if data.Message.From != "" &&
			len(data.Message.From) > len(domain) &&
			data.Message.From[len(data.Message.From)-len(domain):] == domain {

			p.logger.Warn("🚫 Blocking message from blocked domain",
				"from", data.Message.From,
				"domain", domain,
				"messageId", data.Message.MessageID)

			// Set rejection result
			if data.Result == nil {
				data.Result = &plugin.HookResult{}
			}
			data.Result.Reject = true
			data.Result.Message = fmt.Sprintf("Messages from domain %s are not allowed", domain)

			return nil
		}
	}

	p.logger.Info("✅ Message passed filter checks", "from", data.Message.From)
	return nil
}

func main() {
	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create plugin manager
	config := map[string]interface{}{
		"simple-example": map[string]interface{}{
			"enabled": true,
			"debug":   true,
		},
		"demo-filter": map[string]interface{}{
			"enabled": true,
		},
	}

	manager := plugin.NewManager(config, logger)

	// Register example plugins
	simplePlugin := NewSimpleExamplePlugin(logger)
	filterPlugin := NewDemoFilterPlugin(logger)

	if err := manager.RegisterPlugin(simplePlugin); err != nil {
		logger.Error("Failed to register simple plugin", "error", err)
		return
	}

	if err := manager.RegisterPlugin(filterPlugin); err != nil {
		logger.Error("Failed to register filter plugin", "error", err)
		return
	}

	// Create example message
	exampleMessage := &queue.QueuedMessage{
		MessageID:  "example-123",
		From:       "test@example.com",
		Recipients: []string{"user@destination.com"},
		Zone:       "default",
		Interface:  "api",
		Origin:     "127.0.0.1",
		Subject:    "Test Email",
	}

	ctx := context.Background()

	// Simulate message processing through hooks
	logger.Info("🚀 Starting plugin demonstration")

	// 1. Message received
	logger.Info("--- Step 1: Message Received ---")
	hookData := &plugin.HookData{
		Message:   exampleMessage,
		Interface: "api",
		RemoteIP:  "127.0.0.1",
	}

	if err := manager.ExecuteHooks(ctx, plugin.HookReceive, hookData); err != nil {
		logger.Error("Hook execution failed", "hook", "receive", "error", err)
		return
	}

	// 2. Message queued
	logger.Info("--- Step 2: Message Queued ---")
	if err := manager.ExecuteHooks(ctx, plugin.HookQueue, hookData); err != nil {
		logger.Error("Hook execution failed", "hook", "queue", "error", err)
		return
	}

	// 3. Message delivered
	logger.Info("--- Step 3: Message Delivered ---")
	hookData.Message.DeliveryAttempts = 1
	if err := manager.ExecuteHooks(ctx, plugin.HookDelivered, hookData); err != nil {
		logger.Error("Hook execution failed", "hook", "delivered", "error", err)
		return
	}

	logger.Info("--- Testing Filter Plugin ---")

	// Test filter with blocked domain
	blockedMessage := &queue.QueuedMessage{
		MessageID:  "blocked-456",
		From:       "spammer@spam.example.com",
		Recipients: []string{"victim@destination.com"},
		Zone:       "default",
		Interface:  "smtp",
		Origin:     "192.168.1.100",
	}

	blockedHookData := &plugin.HookData{
		Message:   blockedMessage,
		Interface: "smtp",
		RemoteIP:  "192.168.1.100",
	}

	if err := manager.ExecuteHooks(ctx, plugin.HookReceive, blockedHookData); err != nil {
		logger.Info("Message was filtered", "reason", err.Error())
	}

	// Show plugin information
	logger.Info("--- Plugin Information ---")
	plugins := manager.GetPlugins()
	for name, p := range plugins {
		logger.Info("Registered plugin",
			"name", name,
			"description", p.Description(),
			"version", p.Version())
	}

	// Cleanup
	if err := manager.Shutdown(ctx); err != nil {
		logger.Error("Failed to shutdown plugin manager", "error", err)
	}

	logger.Info("🎉 Plugin demonstration completed successfully!")
}
