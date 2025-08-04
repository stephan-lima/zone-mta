package smtp

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/zone-eu/zone-mta-go/internal/config"
	"github.com/zone-eu/zone-mta-go/internal/queue"
)

// Server represents an SMTP server
type Server struct {
	config    config.SMTPInterfaceConfig
	name      string
	queue     *queue.Manager
	logger    *slog.Logger
	listener  net.Listener
	tlsConfig *tls.Config
	shutdown  chan struct{}
	wg        sync.WaitGroup
	mu        sync.RWMutex
	running   bool
}

// NewServer creates a new SMTP server
func NewServer(name string, cfg config.SMTPInterfaceConfig, queueMgr *queue.Manager, logger *slog.Logger) *Server {
	return &Server{
		config:   cfg,
		name:     name,
		queue:    queueMgr,
		logger:   logger.With("component", "smtp-server", "interface", name),
		shutdown: make(chan struct{}),
	}
}

// SetTLSConfig sets the TLS configuration for the server
func (s *Server) SetTLSConfig(tlsConfig *tls.Config) {
	s.tlsConfig = tlsConfig
}

// Start starts the SMTP server
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	var err error
	if s.config.Secure && s.tlsConfig != nil {
		s.listener, err = tls.Listen("tcp", addr, s.tlsConfig)
	} else {
		s.listener, err = net.Listen("tcp", addr)
	}

	if err != nil {
		return fmt.Errorf("failed to start SMTP server on %s: %w", addr, err)
	}

	s.running = true
	s.logger.Info("SMTP server started", "address", addr, "secure", s.config.Secure)

	// Start accepting connections
	s.wg.Add(1)
	go s.acceptConnections()

	return nil
}

// Stop stops the SMTP server
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false
	close(s.shutdown)

	// Close listener
	if s.listener != nil {
		s.listener.Close()
	}

	// Wait for all connections to close
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("SMTP server stopped gracefully")
		return nil
	case <-ctx.Done():
		s.logger.Warn("SMTP server stop timed out")
		return ctx.Err()
	}
}

// acceptConnections accepts incoming connections
func (s *Server) acceptConnections() {
	defer s.wg.Done()

	for {
		select {
		case <-s.shutdown:
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				return
			default:
				s.logger.Error("Failed to accept connection", "error", err)
				continue
			}
		}

		// Handle connection in a separate goroutine
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection handles an individual SMTP connection
func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	// Set connection timeout
	conn.SetDeadline(time.Now().Add(5 * time.Minute))

	// Create SMTP session
	session := &Session{
		server: s,
		conn:   conn,
		reader: textproto.NewReader(bufio.NewReader(conn)),
		writer: textproto.NewWriter(bufio.NewWriter(conn)),
		logger: s.logger.With("remote", conn.RemoteAddr().String()),
	}

	// Handle the session
	session.Handle()
}

// Session represents an SMTP session
type Session struct {
	server *Server
	conn   net.Conn
	reader *textproto.Reader
	writer *textproto.Writer
	logger *slog.Logger

	// Session state
	helo     string
	mailFrom string
	rcptTo   []string
	data     []byte

	// Flags
	authenticated bool
	tlsEnabled    bool
}

// Handle handles the SMTP session
func (s *Session) Handle() {
	s.logger.Info("New SMTP connection")

	// Send greeting
	hostname, _ := net.LookupAddr(s.conn.LocalAddr().(*net.TCPAddr).IP.String())
	if len(hostname) == 0 {
		hostname = []string{s.server.config.Host}
	}

	err := s.writer.PrintfLine("220 %s %s ESMTP ZoneMTA-Go", hostname[0], s.server.name)
	if err != nil {
		s.logger.Error("Failed to send greeting", "error", err)
		return
	}

	// Process commands
	for {
		line, err := s.reader.ReadLine()
		if err != nil {
			s.logger.Debug("Connection closed", "error", err)
			return
		}

		s.logger.Debug("Received command", "command", line)

		// Parse command
		parts := strings.Fields(line)
		if len(parts) == 0 {
			s.sendResponse(500, "Syntax error")
			continue
		}

		cmd := strings.ToUpper(parts[0])
		args := ""
		if len(parts) > 1 {
			args = strings.Join(parts[1:], " ")
		}

		// Handle command
		if err := s.handleCommand(cmd, args); err != nil {
			s.logger.Error("Command failed", "command", cmd, "error", err)
			return
		}
	}
}

// handleCommand handles individual SMTP commands
func (s *Session) handleCommand(cmd, args string) error {
	switch cmd {
	case "HELO":
		return s.handleHelo(args, false)
	case "EHLO":
		return s.handleHelo(args, true)
	case "STARTTLS":
		return s.handleStartTLS()
	case "AUTH":
		return s.handleAuth(args)
	case "MAIL":
		return s.handleMail(args)
	case "RCPT":
		return s.handleRcpt(args)
	case "DATA":
		return s.handleData()
	case "RSET":
		return s.handleRset()
	case "NOOP":
		return s.sendResponse(250, "OK")
	case "QUIT":
		s.sendResponse(221, "Bye")
		return fmt.Errorf("quit")
	default:
		return s.sendResponse(502, "Command not implemented")
	}
}

// handleHelo handles HELO/EHLO commands
func (s *Session) handleHelo(args string, extended bool) error {
	if args == "" {
		return s.sendResponse(501, "Syntax: HELO hostname")
	}

	s.helo = args

	if extended {
		// EHLO response with extensions
		lines := []string{
			fmt.Sprintf("250-%s Hello %s", s.server.name, args),
			"250-SIZE 35651584", // ~34MB
			"250-8BITMIME",
			"250-PIPELINING",
		}

		// Add STARTTLS if not secure and TLS is available
		if !s.server.config.Secure && !s.tlsEnabled && s.server.tlsConfig != nil && !s.server.config.DisableSTARTTLS {
			lines = append(lines, "250-STARTTLS")
		}

		// Add AUTH if authentication is enabled
		if s.server.config.Authentication {
			lines = append(lines, "250-AUTH PLAIN LOGIN")
		}

		lines = append(lines, "250 HELP")

		return s.sendMultilineResponse(lines)
	} else {
		// Simple HELO response
		return s.sendResponse(250, fmt.Sprintf("%s Hello %s", s.server.name, args))
	}
}

// handleStartTLS handles STARTTLS command
func (s *Session) handleStartTLS() error {
	if s.server.config.DisableSTARTTLS || s.server.tlsConfig == nil {
		return s.sendResponse(502, "STARTTLS not available")
	}

	if s.tlsEnabled {
		return s.sendResponse(503, "Already running in TLS")
	}

	if err := s.sendResponse(220, "Ready to start TLS"); err != nil {
		return err
	}

	// Upgrade connection to TLS
	tlsConn := tls.Server(s.conn, s.server.tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		s.logger.Error("TLS handshake failed", "error", err)
		return err
	}

	// Update connection and readers/writers
	s.conn = tlsConn
	s.reader = textproto.NewReader(bufio.NewReader(tlsConn))
	s.writer = textproto.NewWriter(bufio.NewWriter(tlsConn))
	s.tlsEnabled = true

	// Reset session state after STARTTLS
	s.helo = ""
	s.mailFrom = ""
	s.rcptTo = nil

	s.logger.Info("TLS enabled")
	return nil
}

// handleAuth handles AUTH command
func (s *Session) handleAuth(args string) error {
	if !s.server.config.Authentication {
		return s.sendResponse(502, "Authentication not enabled")
	}

	// Simple authentication implementation
	// In a real implementation, this would integrate with an authentication backend
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return s.sendResponse(501, "Syntax: AUTH mechanism")
	}

	mechanism := strings.ToUpper(parts[0])
	switch mechanism {
	case "PLAIN":
		// For simplicity, accept any authentication
		s.authenticated = true
		return s.sendResponse(235, "Authentication successful")
	case "LOGIN":
		// For simplicity, accept any authentication
		s.authenticated = true
		return s.sendResponse(235, "Authentication successful")
	default:
		return s.sendResponse(504, "Authentication mechanism not supported")
	}
}

// handleMail handles MAIL FROM command
func (s *Session) handleMail(args string) error {
	if s.helo == "" {
		return s.sendResponse(503, "Send HELO/EHLO first")
	}

	if s.server.config.Authentication && !s.authenticated {
		return s.sendResponse(530, "Authentication required")
	}

	if !strings.HasPrefix(strings.ToUpper(args), "FROM:") {
		return s.sendResponse(501, "Syntax: MAIL FROM:<address>")
	}

	// Extract email address
	fromPart := args[5:] // Remove "FROM:"
	fromPart = strings.TrimSpace(fromPart)
	if strings.HasPrefix(fromPart, "<") && strings.HasSuffix(fromPart, ">") {
		fromPart = fromPart[1 : len(fromPart)-1]
	}

	s.mailFrom = fromPart
	s.rcptTo = nil // Reset recipients

	return s.sendResponse(250, "OK")
}

// handleRcpt handles RCPT TO command
func (s *Session) handleRcpt(args string) error {
	if s.mailFrom == "" {
		return s.sendResponse(503, "Send MAIL FROM first")
	}

	if !strings.HasPrefix(strings.ToUpper(args), "TO:") {
		return s.sendResponse(501, "Syntax: RCPT TO:<address>")
	}

	// Check recipient limit
	if len(s.rcptTo) >= s.server.config.MaxRecipients {
		return s.sendResponse(452, "Too many recipients")
	}

	// Extract email address
	toPart := args[3:] // Remove "TO:"
	toPart = strings.TrimSpace(toPart)
	if strings.HasPrefix(toPart, "<") && strings.HasSuffix(toPart, ">") {
		toPart = toPart[1 : len(toPart)-1]
	}

	s.rcptTo = append(s.rcptTo, toPart)

	return s.sendResponse(250, "OK")
}

// handleData handles DATA command
func (s *Session) handleData() error {
	if len(s.rcptTo) == 0 {
		return s.sendResponse(503, "Send RCPT TO first")
	}

	if err := s.sendResponse(354, "Start mail input; end with <CRLF>.<CRLF>"); err != nil {
		return err
	}

	// Read message data until ".\r\n"
	data, err := s.reader.ReadDotBytes()
	if err != nil {
		s.logger.Error("Failed to read message data", "error", err)
		return s.sendResponse(451, "Failed to read message")
	}

	s.data = data

	// Process the message
	return s.processMessage()
}

// processMessage processes the received message and queues it
func (s *Session) processMessage() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create queued message
	msg := &queue.QueuedMessage{
		Interface:  s.server.name,
		From:       s.mailFrom,
		Recipients: s.rcptTo,
		Origin:     s.conn.RemoteAddr().(*net.TCPAddr).IP.String(),
		Status:     queue.StatusQueued,
	}

	// Store message in queue
	err := s.server.queue.Store(ctx, msg, strings.NewReader(string(s.data)))
	if err != nil {
		s.logger.Error("Failed to queue message", "error", err)
		return s.sendResponse(451, "Failed to queue message")
	}

	s.logger.Info("Message queued",
		"messageId", msg.MessageID,
		"from", s.mailFrom,
		"recipients", len(s.rcptTo),
		"size", len(s.data))

	// Reset session state
	s.mailFrom = ""
	s.rcptTo = nil
	s.data = nil

	return s.sendResponse(250, "OK: Message queued")
}

// handleRset handles RSET command
func (s *Session) handleRset() error {
	s.mailFrom = ""
	s.rcptTo = nil
	s.data = nil
	return s.sendResponse(250, "OK")
}

// sendResponse sends an SMTP response
func (s *Session) sendResponse(code int, message string) error {
	return s.writer.PrintfLine("%d %s", code, message)
}

// sendMultilineResponse sends a multiline SMTP response
func (s *Session) sendMultilineResponse(lines []string) error {
	for _, line := range lines {
		if err := s.writer.PrintfLine(line); err != nil {
			return err
		}
	}
	return nil
}
