package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zone-eu/zone-mta-go/internal/config"
	"github.com/zone-eu/zone-mta-go/internal/db"
	"github.com/zone-eu/zone-mta-go/internal/plugin"
	"github.com/zone-eu/zone-mta-go/internal/queue"
	"github.com/zone-eu/zone-mta-go/internal/sender"
	socks5proxy "github.com/zone-eu/zone-mta-go/plugins/socks5-proxy"
)

func main() {
	// Set up logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Load configuration
	cfg, err := config.Load("../../config/example-socks5-proxy.yaml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// Initialize database manager
	dbManager := db.NewManager(cfg.Databases, logger)
	ctx := context.Background()
	if err := dbManager.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to databases: %v", err)
	}
	defer dbManager.Close(ctx)

	// Initialize queue manager
	queueManager := queue.NewManager(cfg.Queue, dbManager, logger)

	// Initialize plugin manager
	pluginManager := plugin.NewManager(cfg.Plugins, logger)

	// Register the SOCKS5 proxy plugin
	socks5Plugin := socks5proxy.NewSOCKS5ProxyPlugin(logger)
	if err := pluginManager.RegisterPlugin(socks5Plugin); err != nil {
		log.Fatalf("Failed to register SOCKS5 proxy plugin: %v", err)
	}

	// Load other built-in plugins
	if err := pluginManager.LoadBuiltinPluginsWithDeps(dbManager); err != nil {
		log.Fatalf("Failed to load built-in plugins: %v", err)
	}

	// Initialize sender manager with plugin support
	senderManager := sender.NewSenderManager(queueManager, pluginManager, logger)

	// Add sending zones
	for zoneName, zoneConfig := range cfg.SendingZones {
		senderManager.AddZone(zoneName, zoneConfig)
		logger.Info("Added sending zone", "zone", zoneName)
	}

	// Start sender manager
	if err := senderManager.Start(ctx); err != nil {
		log.Fatalf("Failed to start sender manager: %v", err)
	}

	logger.Info("ZoneMTA-Go started with SOCKS5 proxy support")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down...")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop sender manager
	if err := senderManager.Stop(shutdownCtx); err != nil {
		logger.Error("Failed to stop sender manager", "error", err)
	}

	// Shutdown plugins
	if err := pluginManager.Shutdown(shutdownCtx); err != nil {
		logger.Error("Failed to shutdown plugins", "error", err)
	}

	// Close database connections
	if err := dbManager.Close(shutdownCtx); err != nil {
		logger.Error("Failed to close database connections", "error", err)
	}

	logger.Info("Shutdown complete")
}