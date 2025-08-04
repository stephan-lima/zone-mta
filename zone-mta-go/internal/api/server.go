package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zone-eu/zone-mta-go/internal/config"
	"github.com/zone-eu/zone-mta-go/internal/queue"
)

// Server represents the HTTP API server
type Server struct {
	config config.APIConfig
	queue  *queue.Manager
	logger *slog.Logger
	server *http.Server
	mux    *http.ServeMux
}

// NewServer creates a new API server
func NewServer(cfg config.APIConfig, queueMgr *queue.Manager, logger *slog.Logger) *Server {
	mux := http.NewServeMux()

	s := &Server{
		config: cfg,
		queue:  queueMgr,
		logger: logger.With("component", "api-server"),
		mux:    mux,
	}

	// Register routes
	s.registerRoutes()

	// Create HTTP server
	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      s.loggingMiddleware(s.corsMiddleware(mux)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// registerRoutes registers all API routes
func (s *Server) registerRoutes() {
	// Health check
	s.mux.HandleFunc("/health", s.handleHealth)

	// Queue operations
	s.mux.HandleFunc("/api/v1/queue/send", s.handleSendMail)
	s.mux.HandleFunc("/api/v1/queue/list", s.handleListMessages)
	s.mux.HandleFunc("/api/v1/queue/stats", s.handleQueueStats)
	s.mux.HandleFunc("/api/v1/queue/message/", s.handleMessageOperations)

	// System operations
	s.mux.HandleFunc("/api/v1/system/status", s.handleSystemStatus)
	s.mux.HandleFunc("/api/v1/system/metrics", s.handleMetrics)
}

// Start starts the API server
func (s *Server) Start(ctx context.Context) error {
	if !s.config.Enabled {
		s.logger.Info("API server disabled")
		return nil
	}

	s.logger.Info("Starting API server", "address", s.server.Addr)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("API server failed", "error", err)
		}
	}()

	return nil
}

// Stop stops the API server
func (s *Server) Stop(ctx context.Context) error {
	if !s.config.Enabled {
		return nil
	}

	s.logger.Info("Stopping API server")
	return s.server.Shutdown(ctx)
}

// Middleware

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap the response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		s.logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration", duration,
			"remote", r.RemoteAddr,
			"user_agent", r.UserAgent())
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Handler implementations

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC(),
		"version":   "1.0.0",
	}

	s.writeJSONResponse(w, http.StatusOK, response)
}

func (s *Server) handleSendMail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req SendMailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid JSON", err)
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Validation failed", err)
		return
	}

	// Check recipient limit
	if len(req.To) > s.config.MaxRecipients {
		s.writeErrorResponse(w, http.StatusBadRequest, "Too many recipients",
			fmt.Errorf("maximum %d recipients allowed", s.config.MaxRecipients))
		return
	}

	// Create message
	msg := &queue.QueuedMessage{
		Interface:  "api",
		From:       req.From,
		Recipients: req.To,
		Zone:       req.Zone,
		Origin:     s.getClientIP(r),
		Status:     queue.StatusQueued,
		Subject:    req.Subject,
		Headers:    req.Headers,
		Tags:       req.Tags,
		Metadata:   req.Metadata,
	}

	// Store message
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	content := strings.NewReader(req.Message)
	if err := s.queue.Store(ctx, msg, content); err != nil {
		s.logger.Error("Failed to store message", "error", err)
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to queue message", err)
		return
	}

	response := SendMailResponse{
		MessageID: msg.MessageID,
		Status:    "queued",
		QueueTime: time.Now().UTC(),
	}

	s.writeJSONResponse(w, http.StatusOK, response)
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	filter := queue.QueueFilter{
		Zone:      r.URL.Query().Get("zone"),
		Status:    queue.MessageStatus(r.URL.Query().Get("status")),
		Interface: r.URL.Query().Get("interface"),
		From:      r.URL.Query().Get("from"),
		Recipient: r.URL.Query().Get("recipient"),
	}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			filter.Limit = l
		}
	}

	if skip := r.URL.Query().Get("skip"); skip != "" {
		if s, err := strconv.Atoi(skip); err == nil && s >= 0 {
			filter.Skip = s
		}
	}

	// Query messages
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	messages, err := s.queue.List(ctx, filter)
	if err != nil {
		s.logger.Error("Failed to list messages", "error", err)
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to list messages", err)
		return
	}

	response := map[string]interface{}{
		"messages": messages,
		"count":    len(messages),
		"filter":   filter,
	}

	s.writeJSONResponse(w, http.StatusOK, response)
}

func (s *Server) handleQueueStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	stats, err := s.queue.GetStats(ctx)
	if err != nil {
		s.logger.Error("Failed to get queue stats", "error", err)
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get stats", err)
		return
	}

	s.writeJSONResponse(w, http.StatusOK, stats)
}

func (s *Server) handleMessageOperations(w http.ResponseWriter, r *http.Request) {
	// Extract message ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/queue/message/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Message ID required", http.StatusBadRequest)
		return
	}

	messageID := parts[0]

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		s.handleGetMessage(w, r, messageID, ctx)
	case http.MethodDelete:
		s.handleDeleteMessage(w, r, messageID, ctx)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleGetMessage(w http.ResponseWriter, r *http.Request, messageID string, ctx context.Context) {
	msg, err := s.queue.Get(ctx, messageID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Message not found", http.StatusNotFound)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get message", err)
		}
		return
	}

	// Check if content is requested
	if r.URL.Query().Get("content") == "true" {
		content, err := s.queue.GetContent(ctx, messageID)
		if err != nil {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get message content", err)
			return
		}
		defer content.Close()

		// Stream content back
		w.Header().Set("Content-Type", "message/rfc822")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.eml", messageID))
		io.Copy(w, content)
		return
	}

	s.writeJSONResponse(w, http.StatusOK, msg)
}

func (s *Server) handleDeleteMessage(w http.ResponseWriter, r *http.Request, messageID string, ctx context.Context) {
	if err := s.queue.Delete(ctx, messageID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Message not found", http.StatusNotFound)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete message", err)
		}
		return
	}

	response := map[string]interface{}{
		"messageId": messageID,
		"status":    "deleted",
		"timestamp": time.Now().UTC(),
	}

	s.writeJSONResponse(w, http.StatusOK, response)
}

func (s *Server) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := map[string]interface{}{
		"status":     "running",
		"timestamp":  time.Now().UTC(),
		"uptime":     time.Since(time.Now()), // This would be calculated from startup time
		"version":    "1.0.0",
		"go_version": "go1.21",
	}

	s.writeJSONResponse(w, http.StatusOK, status)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// This would integrate with Prometheus metrics
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("# ZoneMTA-Go Metrics\n# TODO: Implement Prometheus metrics\n"))
}

// Helper methods

func (s *Server) writeJSONResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) writeErrorResponse(w http.ResponseWriter, status int, message string, err error) {
	response := map[string]interface{}{
		"error":     message,
		"status":    status,
		"timestamp": time.Now().UTC(),
	}

	if err != nil {
		response["details"] = err.Error()
	}

	s.writeJSONResponse(w, status, response)
}

func (s *Server) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to remote address
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Request/Response types

type SendMailRequest struct {
	From     string                 `json:"from"`
	To       []string               `json:"to"`
	Subject  string                 `json:"subject,omitempty"`
	Message  string                 `json:"message"`
	Zone     string                 `json:"zone,omitempty"`
	Headers  map[string]string      `json:"headers,omitempty"`
	Tags     []string               `json:"tags,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

func (r *SendMailRequest) Validate() error {
	if r.From == "" {
		return fmt.Errorf("from address is required")
	}
	if len(r.To) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	if r.Message == "" {
		return fmt.Errorf("message content is required")
	}
	return nil
}

type SendMailResponse struct {
	MessageID string    `json:"messageId"`
	Status    string    `json:"status"`
	QueueTime time.Time `json:"queueTime"`
}
