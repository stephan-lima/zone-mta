package ratelimiter

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/zone-eu/zone-mta-go/internal/db"
	"github.com/zone-eu/zone-mta-go/internal/plugin"
)

// RateLimiterPlugin implements rate limiting based on sender address and IP
type RateLimiterPlugin struct {
	redis  *db.Redis
	config *Config
	logger *slog.Logger
}

// Config represents the rate limiter configuration
type Config struct {
	Enabled     bool              `yaml:"enabled"`
	Limits      map[string]string `yaml:"limits"`      // e.g., "sender": "100/hour", "ip": "1000/hour"
	WhitelistIP []string          `yaml:"whitelistIp"` // IP addresses to whitelist
	Action      string            `yaml:"action"`      // "reject" or "defer"
	Message     string            `yaml:"message"`     // Custom rejection message
}

// NewRateLimiterPlugin creates a new rate limiter plugin
func NewRateLimiterPlugin(redis *db.Redis, logger *slog.Logger) *RateLimiterPlugin {
	return &RateLimiterPlugin{
		redis:  redis,
		logger: logger.With("plugin", "rate-limiter"),
	}
}

// Name returns the plugin name
func (p *RateLimiterPlugin) Name() string {
	return "rate-limiter"
}

// Description returns the plugin description
func (p *RateLimiterPlugin) Description() string {
	return "Rate limiting plugin that controls email sending rates per sender and IP"
}

// Version returns the plugin version
func (p *RateLimiterPlugin) Version() string {
	return "1.0.0"
}

// Initialize initializes the plugin with configuration
func (p *RateLimiterPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.config = &Config{
		Enabled: true,
		Limits: map[string]string{
			"sender": "100/hour",
			"ip":     "1000/hour",
		},
		Action:  "defer",
		Message: "Rate limit exceeded, please try again later",
	}

	// Parse configuration
	if enabled, ok := config["enabled"].(bool); ok {
		p.config.Enabled = enabled
	}

	if limits, ok := config["limits"].(map[string]interface{}); ok {
		p.config.Limits = make(map[string]string)
		for key, value := range limits {
			if strValue, ok := value.(string); ok {
				p.config.Limits[key] = strValue
			}
		}
	}

	if whitelist, ok := config["whitelistIp"].([]interface{}); ok {
		p.config.WhitelistIP = make([]string, len(whitelist))
		for i, ip := range whitelist {
			if ipStr, ok := ip.(string); ok {
				p.config.WhitelistIP[i] = ipStr
			}
		}
	}

	if action, ok := config["action"].(string); ok {
		p.config.Action = action
	}

	if message, ok := config["message"].(string); ok {
		p.config.Message = message
	}

	p.logger.Info("Rate limiter plugin initialized",
		"enabled", p.config.Enabled,
		"limits", p.config.Limits,
		"action", p.config.Action)

	return nil
}

// RegisterHooks registers the plugin hooks
func (p *RateLimiterPlugin) RegisterHooks(manager *plugin.Manager) error {
	if !p.config.Enabled {
		return nil
	}

	// Register hooks for different stages
	manager.RegisterHook(plugin.HookReceive, p.checkRateLimit)
	manager.RegisterHook(plugin.HookQueue, p.updateCounters)
	manager.RegisterHook(plugin.HookDelivered, p.cleanupCounters)

	p.logger.Info("Rate limiter hooks registered")
	return nil
}

// Shutdown cleans up the plugin
func (p *RateLimiterPlugin) Shutdown(ctx context.Context) error {
	p.logger.Info("Rate limiter plugin shutting down")
	return nil
}

// checkRateLimit checks if the message should be rate limited
func (p *RateLimiterPlugin) checkRateLimit(ctx context.Context, data *plugin.HookData) error {
	if data.Message == nil {
		return nil
	}

	// Check if IP is whitelisted
	if p.isWhitelisted(data.RemoteIP) {
		p.logger.Debug("IP whitelisted, skipping rate limit", "ip", data.RemoteIP)
		return nil
	}

	// Check rate limits
	for limitType, limitStr := range p.config.Limits {
		if exceeded, err := p.checkLimit(ctx, limitType, data, limitStr); err != nil {
			p.logger.Error("Failed to check rate limit", "type", limitType, "error", err)
			continue
		} else if exceeded {
			p.logger.Warn("Rate limit exceeded",
				"type", limitType,
				"limit", limitStr,
				"sender", data.Message.From,
				"ip", data.RemoteIP)

			// Set rejection/defer result
			if data.Result == nil {
				data.Result = &plugin.HookResult{}
			}

			if p.config.Action == "reject" {
				data.Result.Reject = true
			} else {
				// Defer the message (this would need to be handled by the SMTP server)
				data.Result.Accept = false
			}
			data.Result.Message = p.config.Message

			return nil // Don't return error, just set result
		}
	}

	return nil
}

// updateCounters updates the rate limit counters
func (p *RateLimiterPlugin) updateCounters(ctx context.Context, data *plugin.HookData) error {
	if data.Message == nil {
		return nil
	}

	// Update counters for each limit type
	for limitType, limitStr := range p.config.Limits {
		key := p.getCounterKey(limitType, data)
		if key == "" {
			continue
		}

		duration := p.getLimitDuration(limitStr)
		if duration == 0 {
			continue
		}

		// Increment counter with expiration
		_, err := p.redis.IncrementWithExpire(ctx, key, duration)
		if err != nil {
			p.logger.Error("Failed to update rate limit counter",
				"key", key,
				"error", err)
		}
	}

	return nil
}

// cleanupCounters performs any necessary cleanup after delivery
func (p *RateLimiterPlugin) cleanupCounters(ctx context.Context, data *plugin.HookData) error {
	// This hook can be used for cleanup or statistics
	// For now, we'll just log successful deliveries
	if data.Message != nil {
		p.logger.Debug("Message delivered, rate limit counters maintained",
			"messageId", data.Message.MessageID,
			"sender", data.Message.From)
	}
	return nil
}

// checkLimit checks if a specific limit is exceeded
func (p *RateLimiterPlugin) checkLimit(ctx context.Context, limitType string, data *plugin.HookData, limitStr string) (bool, error) {
	key := p.getCounterKey(limitType, data)
	if key == "" {
		return false, nil
	}

	// Get current count
	countStr, err := p.redis.Get(ctx, key)
	if err != nil {
		// Key doesn't exist, no limit exceeded
		return false, nil
	}

	count, err := strconv.ParseInt(countStr, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid counter value: %w", err)
	}

	// Parse limit
	limit := p.getLimitValue(limitStr)
	if limit == 0 {
		return false, nil
	}

	return count >= limit, nil
}

// getCounterKey generates a Redis key for the rate limit counter
func (p *RateLimiterPlugin) getCounterKey(limitType string, data *plugin.HookData) string {
	switch limitType {
	case "sender":
		if data.Message != nil {
			return fmt.Sprintf("rate_limit:sender:%s:%s",
				data.Message.From,
				p.getTimeWindow(time.Now()))
		}
	case "ip":
		if data.RemoteIP != "" {
			return fmt.Sprintf("rate_limit:ip:%s:%s",
				data.RemoteIP,
				p.getTimeWindow(time.Now()))
		}
	case "interface":
		if data.Interface != "" {
			return fmt.Sprintf("rate_limit:interface:%s:%s",
				data.Interface,
				p.getTimeWindow(time.Now()))
		}
	}
	return ""
}

// getTimeWindow returns a time window string for the current time
func (p *RateLimiterPlugin) getTimeWindow(t time.Time) string {
	// Use hour-based windows for rate limiting
	return t.Format("2006010215") // YYYYMMDDHH
}

// getLimitValue parses limit strings like "100/hour" and returns the numeric limit
func (p *RateLimiterPlugin) getLimitValue(limitStr string) int64 {
	parts := strings.Split(limitStr, "/")
	if len(parts) != 2 {
		return 0
	}

	limit, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0
	}

	return limit
}

// getLimitDuration parses limit strings like "100/hour" and returns the duration
func (p *RateLimiterPlugin) getLimitDuration(limitStr string) time.Duration {
	parts := strings.Split(limitStr, "/")
	if len(parts) != 2 {
		return 0
	}

	switch parts[1] {
	case "second", "sec":
		return time.Second
	case "minute", "min":
		return time.Minute
	case "hour", "hr":
		return time.Hour
	case "day":
		return 24 * time.Hour
	default:
		return 0
	}
}

// isWhitelisted checks if an IP is in the whitelist
func (p *RateLimiterPlugin) isWhitelisted(ip string) bool {
	for _, whiteIP := range p.config.WhitelistIP {
		if ip == whiteIP {
			return true
		}
	}
	return false
}

// GetStats returns rate limiting statistics
func (p *RateLimiterPlugin) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := map[string]interface{}{
		"enabled": p.config.Enabled,
		"limits":  p.config.Limits,
		"action":  p.config.Action,
	}

	// Could add more detailed statistics here
	// like current counter values, blocked messages, etc.

	return stats, nil
}
