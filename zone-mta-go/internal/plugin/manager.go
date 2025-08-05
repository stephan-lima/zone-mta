package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/zone-eu/zone-mta-go/internal/queue"
)

// HookType represents different hook points in the mail processing pipeline
type HookType string

const (
	HookReceive    HookType = "receive"    // When a message is received
	HookQueue      HookType = "queue"      // Before message is queued
	HookPreSend    HookType = "pre_send"   // Before message is sent
	HookPostSend   HookType = "post_send"  // After message is sent
	HookBounce     HookType = "bounce"     // When a message bounces
	HookDelivered  HookType = "delivered"  // When a message is delivered
	HookConnection HookType = "connection" // Before establishing outgoing SMTP connection
)

// Hook represents a function that can be called at specific points
type Hook func(ctx context.Context, data *HookData) error

// HookData contains data passed to hooks
type HookData struct {
	Message    *queue.QueuedMessage   `json:"message"`
	Content    []byte                 `json:"content,omitempty"`
	Error      error                  `json:"error,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Interface  string                 `json:"interface,omitempty"`
	RemoteIP   string                 `json:"remoteIp,omitempty"`
	Result     *HookResult            `json:"result,omitempty"`
	Connection *ConnectionData        `json:"connection,omitempty"`
}

// ConnectionData contains connection-specific data for connection hooks
type ConnectionData struct {
	TargetHost    string `json:"targetHost"`    // Target MX hostname
	TargetPort    string `json:"targetPort"`    // Target port (usually "25")
	LocalIP       string `json:"localIp"`       // Local outgoing IP address
	Zone          string `json:"zone"`          // Sending zone name
	Recipient     string `json:"recipient"`     // Current recipient being delivered to
	MessageID     string `json:"messageId"`     // Message ID being delivered
	DeliveryCount int    `json:"deliveryCount"` // Number of delivery attempts
}

// HookResult contains the result of hook execution
type HookResult struct {
	Accept     bool                   `json:"accept"`
	Reject     bool                   `json:"reject"`
	Message    string                 `json:"message,omitempty"`
	Headers    map[string]string      `json:"headers,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Connection net.Conn              `json:"-"` // Custom connection provided by plugin (not serialized)
}

// Plugin represents a plugin that can register hooks
type Plugin interface {
	Name() string
	Description() string
	Version() string
	Initialize(ctx context.Context, config map[string]interface{}) error
	RegisterHooks(manager *Manager) error
	Shutdown(ctx context.Context) error
}

// Manager manages plugins and their hooks
type Manager struct {
	plugins map[string]Plugin
	hooks   map[HookType][]Hook
	config  map[string]interface{}
	logger  *slog.Logger
	mu      sync.RWMutex
}

// NewManager creates a new plugin manager
func NewManager(config map[string]interface{}, logger *slog.Logger) *Manager {
	return &Manager{
		plugins: make(map[string]Plugin),
		hooks:   make(map[HookType][]Hook),
		config:  config,
		logger:  logger.With("component", "plugin-manager"),
	}
}

// RegisterPlugin registers a plugin
func (m *Manager) RegisterPlugin(plugin Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := plugin.Name()
	if _, exists := m.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}

	// Get plugin-specific configuration
	pluginConfig := make(map[string]interface{})
	if config, ok := m.config[name]; ok {
		if configMap, ok := config.(map[string]interface{}); ok {
			pluginConfig = configMap
		}
	}

	// Initialize plugin
	ctx := context.Background()
	if err := plugin.Initialize(ctx, pluginConfig); err != nil {
		return fmt.Errorf("failed to initialize plugin %s: %w", name, err)
	}

	// Register plugin hooks
	if err := plugin.RegisterHooks(m); err != nil {
		return fmt.Errorf("failed to register hooks for plugin %s: %w", name, err)
	}

	m.plugins[name] = plugin
	m.logger.Info("Plugin registered", "name", name, "version", plugin.Version())

	return nil
}

// RegisterHook registers a hook for a specific hook type
func (m *Manager) RegisterHook(hookType HookType, hook Hook) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.hooks[hookType] = append(m.hooks[hookType], hook)
	m.logger.Debug("Hook registered", "type", hookType)
}

// ExecuteHooks executes all hooks for a specific type
func (m *Manager) ExecuteHooks(ctx context.Context, hookType HookType, data *HookData) error {
	m.mu.RLock()
	hooks := m.hooks[hookType]
	m.mu.RUnlock()

	if len(hooks) == 0 {
		return nil
	}

	m.logger.Debug("Executing hooks", "type", hookType, "count", len(hooks))

	for i, hook := range hooks {
		if err := hook(ctx, data); err != nil {
			m.logger.Error("Hook execution failed",
				"type", hookType,
				"index", i,
				"error", err)
			return fmt.Errorf("hook %d failed: %w", i, err)
		}

		// Check if hook wants to reject/accept
		if data.Result != nil {
			if data.Result.Reject {
				return fmt.Errorf("message rejected by plugin: %s", data.Result.Message)
			}
			if data.Result.Accept {
				break // Stop processing further hooks
			}
		}
	}

	return nil
}

// GetPlugins returns a list of registered plugins
func (m *Manager) GetPlugins() map[string]Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugins := make(map[string]Plugin)
	for name, plugin := range m.plugins {
		plugins[name] = plugin
	}
	return plugins
}

// Shutdown shuts down all plugins
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for name, plugin := range m.plugins {
		if err := plugin.Shutdown(ctx); err != nil {
			m.logger.Error("Failed to shutdown plugin", "name", name, "error", err)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to shutdown %d plugins", len(errs))
	}

	m.logger.Info("All plugins shut down")
	return nil
}

// DefaultHeadersPlugin adds default headers to messages
type DefaultHeadersPlugin struct {
	config map[string]interface{}
	logger *slog.Logger
}

// NewDefaultHeadersPlugin creates a new default headers plugin
func NewDefaultHeadersPlugin(logger *slog.Logger) *DefaultHeadersPlugin {
	return &DefaultHeadersPlugin{
		logger: logger.With("plugin", "default-headers"),
	}
}

func (p *DefaultHeadersPlugin) Name() string        { return "default-headers" }
func (p *DefaultHeadersPlugin) Description() string { return "Adds default headers to messages" }
func (p *DefaultHeadersPlugin) Version() string     { return "1.0.0" }

func (p *DefaultHeadersPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.config = config
	p.logger.Info("Default headers plugin initialized")
	return nil
}

func (p *DefaultHeadersPlugin) RegisterHooks(manager *Manager) error {
	manager.RegisterHook(HookQueue, p.addDefaultHeaders)
	return nil
}

func (p *DefaultHeadersPlugin) Shutdown(ctx context.Context) error {
	p.logger.Info("Default headers plugin shutdown")
	return nil
}

func (p *DefaultHeadersPlugin) addDefaultHeaders(ctx context.Context, data *HookData) error {
	if data.Message == nil {
		return nil
	}

	// Add Message-ID if missing
	if data.Message.Headers == nil {
		data.Message.Headers = make(map[string]string)
	}

	if _, exists := data.Message.Headers["Message-ID"]; !exists {
		data.Message.Headers["Message-ID"] = fmt.Sprintf("<%s@zone-mta-go>", data.Message.MessageID)
	}

	// Add Date if missing
	if _, exists := data.Message.Headers["Date"]; !exists {
		data.Message.Headers["Date"] = data.Message.Created.Format("Mon, 02 Jan 2006 15:04:05 -0700")
	}

	// Add X-Originating-IP if configured and available
	if p.shouldAddOriginatingIP() && data.RemoteIP != "" {
		data.Message.Headers["X-Originating-IP"] = data.RemoteIP
	}

	p.logger.Debug("Added default headers", "messageId", data.Message.MessageID)
	return nil
}

func (p *DefaultHeadersPlugin) shouldAddOriginatingIP() bool {
	if p.config == nil {
		return true
	}

	if val, ok := p.config["xOriginatingIP"]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}

	return true
}

// Registry for built-in plugins
var BuiltinPlugins = map[string]func(*slog.Logger) Plugin{
	"default-headers": func(logger *slog.Logger) Plugin {
		return NewDefaultHeadersPlugin(logger)
	},
}

// LoadBuiltinPlugins loads all built-in plugins
func (m *Manager) LoadBuiltinPlugins() error {
	for name, factory := range BuiltinPlugins {
		// Check if plugin is enabled in config
		if config, ok := m.config[name]; ok {
			if configMap, ok := config.(map[string]interface{}); ok {
				if enabled, ok := configMap["enabled"]; ok {
					if enabledBool, ok := enabled.(bool); ok && !enabledBool {
						m.logger.Info("Plugin disabled in configuration", "name", name)
						continue
					}
				}
			}
		}

		plugin := factory(m.logger)
		if err := m.RegisterPlugin(plugin); err != nil {
			m.logger.Error("Failed to register builtin plugin", "name", name, "error", err)
			return err
		}
	}

	return nil
}

// PluginInfo represents information about a plugin
type PluginInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Enabled     bool   `json:"enabled"`
}

// GetPluginInfo returns information about all registered plugins
func (m *Manager) GetPluginInfo() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var info []PluginInfo
	for _, plugin := range m.plugins {
		info = append(info, PluginInfo{
			Name:        plugin.Name(),
			Description: plugin.Description(),
			Version:     plugin.Version(),
			Enabled:     true,
		})
	}

	return info
}
