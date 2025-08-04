package plugin

import (
	"log/slog"

	"github.com/zone-eu/zone-mta-go/internal/db"
)

// ExternalPluginFactory creates external plugins that need dependencies
type ExternalPluginFactory func(*db.Manager, *slog.Logger) Plugin

// BuiltinPluginFactory creates simple built-in plugins
type BuiltinPluginFactory func(*slog.Logger) Plugin

// Additional built-in plugins registry (extend the main one)
var AdditionalBuiltinPlugins = map[string]BuiltinPluginFactory{
	// Message logger plugin can be imported if the package is included
	// "message-logger": func(logger *slog.Logger) Plugin {
	//     return messagelogger.NewMessageLoggerPlugin(logger)
	// },
}

// Registry for external plugins that need database dependencies
var ExternalPlugins = map[string]ExternalPluginFactory{
	// Rate limiter plugin can be imported if the package is included
	// "rate-limiter": func(dbMgr *db.Manager, logger *slog.Logger) Plugin {
	//     return ratelimiter.NewRateLimiterPlugin(dbMgr.Redis, logger)
	// },
}

// LoadBuiltinPluginsWithDeps loads all built-in plugins with database dependencies
func (m *Manager) LoadBuiltinPluginsWithDeps(dbManager *db.Manager) error {
	// Load simple built-in plugins
	if err := m.LoadBuiltinPlugins(); err != nil {
		return err
	}

	// Load external plugins that need dependencies
	for name, factory := range ExternalPlugins {
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

		plugin := factory(dbManager, m.logger)
		if err := m.RegisterPlugin(plugin); err != nil {
			m.logger.Error("Failed to register external plugin", "name", name, "error", err)
			return err
		}
	}

	return nil
}
