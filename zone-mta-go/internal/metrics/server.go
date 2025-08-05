package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/zone-eu/zone-mta-go/internal/config"
)

// Server represents the Prometheus metrics server
type Server struct {
	config config.MetricsConfig
	logger *slog.Logger
	server *http.Server

	// Custom metrics collectors
	collectors *Collectors
}

// Collectors holds all Prometheus metrics collectors
type Collectors struct {
	// Mail processing metrics
	MailsReceived  *prometheus.CounterVec
	MailsQueued    *prometheus.CounterVec
	MailsDelivered *prometheus.CounterVec
	MailsBounced   *prometheus.CounterVec
	MailsDeferred  *prometheus.CounterVec

	// Queue metrics
	QueueSize        *prometheus.GaugeVec
	QueueAge         *prometheus.HistogramVec
	DeliveryDuration *prometheus.HistogramVec

	// SMTP metrics
	SMTPConnections *prometheus.GaugeVec
	SMTPCommands    *prometheus.CounterVec

	// System metrics
	DBConnections  *prometheus.GaugeVec
	ProcessingTime *prometheus.HistogramVec
	ErrorCount     *prometheus.CounterVec

	// Rate limiting metrics
	RateLimitHits *prometheus.CounterVec
}

// NewServer creates a new metrics server
func NewServer(cfg config.MetricsConfig, logger *slog.Logger) *Server {
	collectors := &Collectors{
		// Mail processing metrics
		MailsReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "zonemta_mails_received_total",
				Help: "Total number of emails received",
			},
			[]string{"interface", "zone"},
		),
		MailsQueued: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "zonemta_mails_queued_total",
				Help: "Total number of emails queued for delivery",
			},
			[]string{"zone"},
		),
		MailsDelivered: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "zonemta_mails_delivered_total",
				Help: "Total number of emails successfully delivered",
			},
			[]string{"zone"},
		),
		MailsBounced: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "zonemta_mails_bounced_total",
				Help: "Total number of emails that bounced",
			},
			[]string{"zone", "reason"},
		),
		MailsDeferred: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "zonemta_mails_deferred_total",
				Help: "Total number of emails deferred for retry",
			},
			[]string{"zone"},
		),

		// Queue metrics
		QueueSize: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "zonemta_queue_size",
				Help: "Current number of emails in queue",
			},
			[]string{"zone", "status"},
		),
		QueueAge: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "zonemta_queue_age_seconds",
				Help:    "Age of emails in queue",
				Buckets: prometheus.ExponentialBuckets(1, 2, 15), // 1s to ~9h
			},
			[]string{"zone"},
		),
		DeliveryDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "zonemta_delivery_duration_seconds",
				Help:    "Time taken to deliver emails",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 100ms to ~2min
			},
			[]string{"zone"},
		),

		// SMTP metrics
		SMTPConnections: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "zonemta_smtp_connections",
				Help: "Current number of SMTP connections",
			},
			[]string{"interface", "type"}, // type: inbound, outbound
		),
		SMTPCommands: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "zonemta_smtp_commands_total",
				Help: "Total number of SMTP commands processed",
			},
			[]string{"interface", "command", "status"},
		),

		// System metrics
		DBConnections: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "zonemta_db_connections",
				Help: "Current number of database connections",
			},
			[]string{"database"}, // mongodb, redis
		),
		ProcessingTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "zonemta_processing_duration_seconds",
				Help:    "Time taken for various processing operations",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation"},
		),
		ErrorCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "zonemta_errors_total",
				Help: "Total number of errors by type",
			},
			[]string{"component", "type"},
		),

		// Rate limiting metrics
		RateLimitHits: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "zonemta_rate_limit_hits_total",
				Help: "Total number of rate limit hits",
			},
			[]string{"type", "action"}, // type: sender/ip/interface, action: defer/reject
		),
	}

	return &Server{
		config:     cfg,
		logger:     logger.With("component", "metrics-server"),
		collectors: collectors,
	}
}

// Start starts the metrics server
func (s *Server) Start(ctx context.Context) error {
	if !s.config.Enabled {
		s.logger.Info("Metrics server disabled")
		return nil
	}

	// Register all metrics with Prometheus
	s.registerMetrics()

	// Create HTTP server
	mux := http.NewServeMux()
	mux.Handle(s.config.Path, promhttp.Handler())

	// Add health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      s.loggingMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("Starting metrics server", "port", s.config.Port, "path", s.config.Path)

	// Start server in goroutine
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Metrics server failed", "error", err)
		}
	}()

	return nil
}

// Stop stops the metrics server
func (s *Server) Stop(ctx context.Context) error {
	if !s.config.Enabled || s.server == nil {
		return nil
	}

	s.logger.Info("Stopping metrics server")
	return s.server.Shutdown(ctx)
}

// registerMetrics registers all custom metrics with Prometheus
func (s *Server) registerMetrics() {
	prometheus.MustRegister(
		s.collectors.MailsReceived,
		s.collectors.MailsQueued,
		s.collectors.MailsDelivered,
		s.collectors.MailsBounced,
		s.collectors.MailsDeferred,
		s.collectors.QueueSize,
		s.collectors.QueueAge,
		s.collectors.DeliveryDuration,
		s.collectors.SMTPConnections,
		s.collectors.SMTPCommands,
		s.collectors.DBConnections,
		s.collectors.ProcessingTime,
		s.collectors.ErrorCount,
		s.collectors.RateLimitHits,
	)

	s.logger.Info("Prometheus metrics registered")
}

// loggingMiddleware adds request logging
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		next.ServeHTTP(w, r)

		duration := time.Since(start)
		s.logger.Debug("Metrics request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", duration,
			"remote", r.RemoteAddr)
	})
}

// GetCollectors returns the metrics collectors for use by other components
func (s *Server) GetCollectors() *Collectors {
	return s.collectors
}

// Helper methods for updating metrics

// RecordMailReceived records a received email
func (c *Collectors) RecordMailReceived(interfaceName, zone string) {
	c.MailsReceived.WithLabelValues(interfaceName, zone).Inc()
}

// RecordMailQueued records a queued email
func (c *Collectors) RecordMailQueued(zone string) {
	c.MailsQueued.WithLabelValues(zone).Inc()
}

// RecordMailDelivered records a delivered email
func (c *Collectors) RecordMailDelivered(zone string) {
	c.MailsDelivered.WithLabelValues(zone).Inc()
}

// RecordMailBounced records a bounced email
func (c *Collectors) RecordMailBounced(zone, reason string) {
	c.MailsBounced.WithLabelValues(zone, reason).Inc()
}

// RecordMailDeferred records a deferred email
func (c *Collectors) RecordMailDeferred(zone string) {
	c.MailsDeferred.WithLabelValues(zone).Inc()
}

// UpdateQueueSize updates the queue size gauge
func (c *Collectors) UpdateQueueSize(zone, status string, size float64) {
	c.QueueSize.WithLabelValues(zone, status).Set(size)
}

// RecordQueueAge records how long an email has been in queue
func (c *Collectors) RecordQueueAge(zone string, age time.Duration) {
	c.QueueAge.WithLabelValues(zone).Observe(age.Seconds())
}

// RecordDeliveryDuration records how long delivery took
func (c *Collectors) RecordDeliveryDuration(zone string, duration time.Duration) {
	c.DeliveryDuration.WithLabelValues(zone).Observe(duration.Seconds())
}

// UpdateSMTPConnections updates SMTP connection count
func (c *Collectors) UpdateSMTPConnections(interfaceName, connType string, count float64) {
	c.SMTPConnections.WithLabelValues(interfaceName, connType).Set(count)
}

// RecordSMTPCommand records an SMTP command
func (c *Collectors) RecordSMTPCommand(interfaceName, command, status string) {
	c.SMTPCommands.WithLabelValues(interfaceName, command, status).Inc()
}

// UpdateDBConnections updates database connection count
func (c *Collectors) UpdateDBConnections(database string, count float64) {
	c.DBConnections.WithLabelValues(database).Set(count)
}

// RecordProcessingTime records processing duration
func (c *Collectors) RecordProcessingTime(operation string, duration time.Duration) {
	c.ProcessingTime.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordError records an error
func (c *Collectors) RecordError(component, errorType string) {
	c.ErrorCount.WithLabelValues(component, errorType).Inc()
}

// RecordRateLimitHit records a rate limit hit
func (c *Collectors) RecordRateLimitHit(limitType, action string) {
	c.RateLimitHits.WithLabelValues(limitType, action).Inc()
}
