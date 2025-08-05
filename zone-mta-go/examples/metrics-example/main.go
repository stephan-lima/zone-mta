package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/zone-eu/zone-mta-go/internal/config"
	"github.com/zone-eu/zone-mta-go/internal/metrics"
)

func main() {
	// Setup logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create metrics configuration
	metricsConfig := config.MetricsConfig{
		Enabled: true,
		Port:    9090,
		Path:    "/metrics",
	}

	// Create metrics server
	logger.Info("Starting metrics server example...")
	metricsServer := metrics.NewServer(metricsConfig, logger)

	// Start the server
	ctx := context.Background()
	if err := metricsServer.Start(ctx); err != nil {
		logger.Error("Failed to start metrics server", "error", err)
		os.Exit(1)
	}

	// Get collectors to simulate some metrics
	collectors := metricsServer.GetCollectors()

	// Simulate some email processing metrics
	logger.Info("Simulating email processing metrics...")

	// Simulate receiving emails
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		zones := []string{"default", "marketing", "transactional"}
		interfaces := []string{"smtp", "api"}

		for {
			select {
			case <-ticker.C:
				// Record some received emails
				for _, zone := range zones {
					for _, iface := range interfaces {
						collectors.RecordMailReceived(iface, zone)
						collectors.RecordMailQueued(zone)
					}
				}

				// Update queue size
				collectors.UpdateQueueSize("default", "queued", 150)
				collectors.UpdateQueueSize("marketing", "queued", 75)
				collectors.UpdateQueueSize("transactional", "queued", 25)

				// Record some deliveries
				collectors.RecordMailDelivered("default")
				collectors.RecordMailDelivered("marketing")

				// Record some bounces occasionally
				if time.Now().Unix()%10 == 0 {
					collectors.RecordMailBounced("default", "user_unknown")
				}

				// Record delivery times
				collectors.RecordDeliveryDuration("default", 2*time.Second)
				collectors.RecordDeliveryDuration("marketing", 1500*time.Millisecond)

				// Record SMTP commands
				collectors.RecordSMTPCommand("smtp", "MAIL", "success")
				collectors.RecordSMTPCommand("smtp", "RCPT", "success")
				collectors.RecordSMTPCommand("smtp", "DATA", "success")

				// Update connection counts
				collectors.UpdateSMTPConnections("smtp", "inbound", 15)
				collectors.UpdateSMTPConnections("api", "outbound", 8)

				// Record database connections
				collectors.UpdateDBConnections("mongodb", 10)
				collectors.UpdateDBConnections("redis", 5)

				logger.Info("Updated metrics", "timestamp", time.Now().Format(time.RFC3339))
			}
		}
	}()

	// Print information about the metrics server
	fmt.Printf(`
🎯 Metrics Server Example Running!

Metrics endpoint: http://localhost:9090/metrics
Health check:     http://localhost:9090/health

Example Prometheus queries:
- Rate of emails received: rate(zonemta_mails_received_total[1m])
- Queue size by zone:      zonemta_queue_size
- Delivery duration p95:   histogram_quantile(0.95, zonemta_delivery_duration_seconds_bucket)
- Error rate:             rate(zonemta_errors_total[5m])

Try these commands:
curl http://localhost:9090/metrics | grep zonemta
curl http://localhost:9090/health

Press Ctrl+C to stop...
`)

	// Wait for interrupt
	select {}
}
