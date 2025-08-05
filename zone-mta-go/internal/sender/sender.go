package sender

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/zone-eu/zone-mta-go/internal/config"
	"github.com/zone-eu/zone-mta-go/internal/plugin"
	"github.com/zone-eu/zone-mta-go/internal/queue"
)

// Sender handles mail delivery
type Sender struct {
	queue         *queue.Manager
	config        config.SendingZoneConfig
	zone          string
	logger        *slog.Logger
	workers       []*Worker
	shutdown      chan struct{}
	wg            sync.WaitGroup
	running       bool
	mu            sync.RWMutex
	pluginManager *plugin.Manager
}

// NewSender creates a new mail sender for a specific zone
func NewSender(zone string, cfg config.SendingZoneConfig, queueMgr *queue.Manager, pluginMgr *plugin.Manager, logger *slog.Logger) *Sender {
	return &Sender{
		queue:         queueMgr,
		config:        cfg,
		zone:          zone,
		logger:        logger.With("component", "sender", "zone", zone),
		shutdown:      make(chan struct{}),
		pluginManager: pluginMgr,
	}
}

// Start starts the sender with configured number of workers
func (s *Sender) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("sender already running")
	}

	s.running = true
	s.logger.Info("Starting sender", "workers", s.config.Connections)

	// Start worker goroutines
	for i := 0; i < s.config.Connections; i++ {
		worker := &Worker{
			id:     fmt.Sprintf("%s-worker-%d", s.zone, i),
			sender: s,
			logger: s.logger.With("worker", i),
		}
		s.workers = append(s.workers, worker)

		s.wg.Add(1)
		go worker.Run()
	}

	return nil
}

// Stop stops the sender and all its workers
func (s *Sender) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	close(s.shutdown)

	// Wait for workers to finish
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("Sender stopped gracefully")
		return nil
	case <-ctx.Done():
		s.logger.Warn("Sender stop timed out")
		return ctx.Err()
	}
}

// Worker represents a delivery worker
type Worker struct {
	id     string
	sender *Sender
	logger *slog.Logger
}

// Run runs the worker loop
func (w *Worker) Run() {
	defer w.sender.wg.Done()

	w.logger.Info("Worker started")
	defer w.logger.Info("Worker stopped")

	ticker := time.NewTicker(5 * time.Second) // Poll every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-w.sender.shutdown:
			return
		case <-ticker.C:
			w.processMessages()
		}
	}
}

// processMessages processes messages from the queue
func (w *Worker) processMessages() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get next message to deliver
	msg, err := w.sender.queue.GetNext(ctx, w.sender.zone, w.id)
	if err != nil {
		w.logger.Error("Failed to get next message", "error", err)
		return
	}

	if msg == nil {
		// No messages to process
		return
	}

	w.logger.Info("Processing message", "messageId", msg.MessageID, "recipients", len(msg.Recipients))

	// Get message content
	content, err := w.sender.queue.GetContent(ctx, msg.MessageID)
	if err != nil {
		w.logger.Error("Failed to get message content", "messageId", msg.MessageID, "error", err)
		w.sender.queue.UpdateStatus(ctx, msg.MessageID, queue.StatusBounced, err.Error())
		return
	}
	defer content.Close()

	// Read content into memory (for multiple recipients)
	var contentBytes []byte
	buffer := make([]byte, 4096)
	for {
		n, err := content.Read(buffer)
		if n > 0 {
			contentBytes = append(contentBytes, buffer[:n]...)
		}
		if err != nil {
			break
		}
	}

	// Deliver to each recipient
	for _, recipient := range msg.Recipients {
		attempt := queue.DeliveryAttempt{
			Timestamp:  time.Now(),
			Recipient:  recipient,
			RetryCount: msg.DeliveryAttempts,
		}

		// Deliver the message
		duration, err := w.deliverToRecipient(ctx, msg, recipient, contentBytes)
		attempt.Duration = duration.Milliseconds()

		if err != nil {
			// Check if it's a permanent or temporary failure
			if isPermanentError(err) {
				attempt.Status = "bounced"
				attempt.Response = err.Error()
				w.logger.Warn("Message bounced", "messageId", msg.MessageID, "recipient", recipient, "error", err)
			} else {
				attempt.Status = "deferred"
				attempt.Response = err.Error()
				w.logger.Info("Message deferred", "messageId", msg.MessageID, "recipient", recipient, "error", err)
			}
		} else {
			attempt.Status = "delivered"
			attempt.Response = "250 OK"
			w.logger.Info("Message delivered", "messageId", msg.MessageID, "recipient", recipient)
		}

		// Record the delivery attempt
		if err := w.sender.queue.RecordDeliveryAttempt(ctx, msg.MessageID, attempt); err != nil {
			w.logger.Error("Failed to record delivery attempt", "messageId", msg.MessageID, "error", err)
		}
	}
}

// deliverToRecipient delivers a message to a specific recipient
func (w *Worker) deliverToRecipient(ctx context.Context, msg *queue.QueuedMessage, recipient string, content []byte) (time.Duration, error) {
	start := time.Now()

	// Extract domain from recipient
	parts := strings.Split(recipient, "@")
	if len(parts) != 2 {
		return time.Since(start), fmt.Errorf("invalid recipient address: %s", recipient)
	}
	domain := parts[1]

	// Look up MX records
	mxRecords, err := net.LookupMX(domain)
	if err != nil {
		return time.Since(start), fmt.Errorf("MX lookup failed for %s: %w", domain, err)
	}

	if len(mxRecords) == 0 {
		return time.Since(start), fmt.Errorf("no MX records found for %s", domain)
	}

	// Try each MX record in order of preference
	var lastErr error
	for _, mx := range mxRecords {
		duration, err := w.deliverViaMX(ctx, mx.Host, msg, recipient, content)
		if err == nil {
			return duration, nil
		}
		lastErr = err
		w.logger.Debug("MX delivery failed", "mx", mx.Host, "error", err)
	}

	return time.Since(start), fmt.Errorf("delivery failed to all MX servers: %w", lastErr)
}

// deliverViaMX delivers a message via a specific MX server
func (w *Worker) deliverViaMX(ctx context.Context, mxHost string, msg *queue.QueuedMessage, recipient string, content []byte) (time.Duration, error) {
	start := time.Now()

	// Set up connection timeout
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Connect to MX server (with plugin hook support)
	conn, err := w.connectWithHooks(ctx, mxHost, "25", msg, recipient)
	if err != nil {
		return time.Since(start), fmt.Errorf("failed to connect to %s: %w", mxHost, err)
	}
	defer conn.Close()

	// Set connection deadline
	conn.SetDeadline(time.Now().Add(60 * time.Second))

	// Create SMTP client
	client, err := smtp.NewClient(conn, mxHost)
	if err != nil {
		return time.Since(start), fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Quit()

	// Check if server supports TLS
	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName:         mxHost,
			InsecureSkipVerify: false, // In production, use proper certificate validation
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			w.logger.Warn("STARTTLS failed", "mx", mxHost, "error", err)
			// Continue without TLS if STARTTLS fails
		}
	}

	// Set sender
	if err := client.Mail(msg.From); err != nil {
		return time.Since(start), fmt.Errorf("MAIL FROM failed: %w", err)
	}

	// Set recipient
	if err := client.Rcpt(recipient); err != nil {
		return time.Since(start), fmt.Errorf("RCPT TO failed: %w", err)
	}

	// Send data
	dataWriter, err := client.Data()
	if err != nil {
		return time.Since(start), fmt.Errorf("DATA command failed: %w", err)
	}

	_, err = dataWriter.Write(content)
	if err != nil {
		dataWriter.Close()
		return time.Since(start), fmt.Errorf("failed to write message data: %w", err)
	}

	err = dataWriter.Close()
	if err != nil {
		return time.Since(start), fmt.Errorf("failed to close data writer: %w", err)
	}

	return time.Since(start), nil
}

// connectWithHooks establishes a connection using plugin hooks for customization
func (w *Worker) connectWithHooks(ctx context.Context, host, port string, msg *queue.QueuedMessage, recipient string) (net.Conn, error) {
	// Get local IP that would be used for this connection
	localIP, err := w.getLocalIP(host, port)
	if err != nil {
		localIP = "unknown" // Continue even if we can't determine local IP
	}

	// Create hook data with connection information
	hookData := &plugin.HookData{
		Message: msg,
		Connection: &plugin.ConnectionData{
			TargetHost:    host,
			TargetPort:    port,
			LocalIP:       localIP,
			Zone:          w.sender.zone,
			Recipient:     recipient,
			MessageID:     msg.MessageID,
			DeliveryCount: msg.DeliveryAttempts,
		},
		Result: &plugin.HookResult{},
	}

	// Execute connection hooks
	if w.sender.pluginManager != nil {
		if err := w.sender.pluginManager.ExecuteHooks(ctx, plugin.HookConnection, hookData); err != nil {
			return nil, fmt.Errorf("connection hook failed: %w", err)
		}

		// Check if a plugin provided a custom connection
		if hookData.Result.Connection != nil {
			w.sender.logger.Debug("Using plugin-provided connection", "host", host, "port", port)
			return hookData.Result.Connection, nil
		}
	}

	// Fall back to default connection
	w.sender.logger.Debug("Using default connection", "host", host, "port", port)
	return net.DialTimeout("tcp", net.JoinHostPort(host, port), 30*time.Second)
}

// getLocalIP determines the local IP that would be used for connecting to the target
func (w *Worker) getLocalIP(targetHost, targetPort string) (string, error) {
	// Create a test connection to determine the local IP
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(targetHost, targetPort), 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr()
	if tcpAddr, ok := localAddr.(*net.TCPAddr); ok {
		return tcpAddr.IP.String(), nil
	}

	return localAddr.String(), nil
}

// isPermanentError checks if an error is a permanent delivery failure
func isPermanentError(err error) bool {
	errStr := strings.ToLower(err.Error())

	// Common permanent error patterns
	permanentPatterns := []string{
		"550", // Permanent failure
		"551", // User not local
		"552", // Exceeded storage allocation
		"553", // Mailbox name not allowed
		"554", // Transaction failed
		"recipient rejected",
		"user unknown",
		"mailbox unavailable",
		"invalid recipient",
		"no such user",
		"user not found",
		"account disabled",
		"mailbox disabled",
	}

	for _, pattern := range permanentPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// SenderManager manages multiple senders for different zones
type SenderManager struct {
	senders       map[string]*Sender
	queue         *queue.Manager
	logger        *slog.Logger
	mu            sync.RWMutex
	pluginManager *plugin.Manager
}

// NewSenderManager creates a new sender manager
func NewSenderManager(queueMgr *queue.Manager, pluginMgr *plugin.Manager, logger *slog.Logger) *SenderManager {
	return &SenderManager{
		senders:       make(map[string]*Sender),
		queue:         queueMgr,
		logger:        logger.With("component", "sender-manager"),
		pluginManager: pluginMgr,
	}
}

// AddZone adds a sending zone
func (sm *SenderManager) AddZone(zone string, cfg config.SendingZoneConfig) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sender := NewSender(zone, cfg, sm.queue, sm.pluginManager, sm.logger)
	sm.senders[zone] = sender
}

// Start starts all senders
func (sm *SenderManager) Start(ctx context.Context) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	for zone, sender := range sm.senders {
		if err := sender.Start(ctx); err != nil {
			sm.logger.Error("Failed to start sender", "zone", zone, "error", err)
			return err
		}
	}

	sm.logger.Info("All senders started", "zones", len(sm.senders))
	return nil
}

// Stop stops all senders
func (sm *SenderManager) Stop(ctx context.Context) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var errs []error
	for zone, sender := range sm.senders {
		if err := sender.Stop(ctx); err != nil {
			sm.logger.Error("Failed to stop sender", "zone", zone, "error", err)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to stop %d senders", len(errs))
	}

	sm.logger.Info("All senders stopped")
	return nil
}
