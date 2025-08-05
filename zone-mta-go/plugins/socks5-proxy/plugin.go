package socks5proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"

	"golang.org/x/net/proxy"
	"github.com/zone-eu/zone-mta-go/internal/plugin"
)

// SOCKS5ProxyPlugin provides SOCKS5 proxy support for outgoing connections
type SOCKS5ProxyPlugin struct {
	config map[string]interface{}
	logger *slog.Logger
}

// ProxyRule represents a rule for when to use a proxy
type ProxyRule struct {
	// Conditions for applying this rule
	Zone      string `json:"zone,omitempty"`      // Specific zone name (empty = all zones)
	Recipient string `json:"recipient,omitempty"` // Recipient pattern (empty = all recipients)
	Host      string `json:"host,omitempty"`      // Target host pattern (empty = all hosts)

	// Proxy configuration
	ProxyHost string `json:"proxyHost"`
	ProxyPort int    `json:"proxyPort"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	Enabled   bool   `json:"enabled"`
}

// NewSOCKS5ProxyPlugin creates a new SOCKS5 proxy plugin
func NewSOCKS5ProxyPlugin(logger *slog.Logger) *SOCKS5ProxyPlugin {
	return &SOCKS5ProxyPlugin{
		logger: logger.With("plugin", "socks5-proxy"),
	}
}

func (p *SOCKS5ProxyPlugin) Name() string {
	return "socks5-proxy"
}

func (p *SOCKS5ProxyPlugin) Description() string {
	return "Provides SOCKS5 proxy support for outgoing SMTP connections"
}

func (p *SOCKS5ProxyPlugin) Version() string {
	return "1.0.0"
}

func (p *SOCKS5ProxyPlugin) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.config = config
	p.logger.Info("SOCKS5 proxy plugin initialized", "config", config)
	return nil
}

func (p *SOCKS5ProxyPlugin) RegisterHooks(manager *plugin.Manager) error {
	manager.RegisterHook(plugin.HookConnection, p.handleConnection)
	return nil
}

func (p *SOCKS5ProxyPlugin) Shutdown(ctx context.Context) error {
	p.logger.Info("SOCKS5 proxy plugin shutdown")
	return nil
}

func (p *SOCKS5ProxyPlugin) handleConnection(ctx context.Context, data *plugin.HookData) error {
	if data.Connection == nil {
		return nil
	}

	// Get the proxy rule for this connection
	rule := p.getProxyRule(data.Connection)
	if rule == nil || !rule.Enabled {
		// No proxy rule applies, let default connection proceed
		return nil
	}

	p.logger.Info("Applying SOCKS5 proxy rule",
		"targetHost", data.Connection.TargetHost,
		"targetPort", data.Connection.TargetPort,
		"proxyHost", rule.ProxyHost,
		"proxyPort", rule.ProxyPort,
		"zone", data.Connection.Zone,
		"recipient", data.Connection.Recipient,
		"localIP", data.Connection.LocalIP)

	// Create SOCKS5 dialer
	var auth *proxy.Auth
	if rule.Username != "" {
		auth = &proxy.Auth{
			User:     rule.Username,
			Password: rule.Password,
		}
	}

	dialer, err := proxy.SOCKS5("tcp",
		net.JoinHostPort(rule.ProxyHost, strconv.Itoa(rule.ProxyPort)),
		auth,
		proxy.Direct)
	if err != nil {
		return fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
	}

	// Establish connection through proxy
	conn, err := dialer.Dial("tcp", net.JoinHostPort(data.Connection.TargetHost, data.Connection.TargetPort))
	if err != nil {
		return fmt.Errorf("failed to connect through SOCKS5 proxy: %w", err)
	}

	// Provide the connection to the sender
	data.Result.Connection = conn

	p.logger.Debug("Successfully established SOCKS5 proxy connection",
		"targetHost", data.Connection.TargetHost,
		"targetPort", data.Connection.TargetPort,
		"proxyHost", rule.ProxyHost,
		"proxyPort", rule.ProxyPort)

	return nil
}

func (p *SOCKS5ProxyPlugin) getProxyRule(conn *plugin.ConnectionData) *ProxyRule {
	if p.config == nil {
		return nil
	}

	// Look for rules in configuration
	rulesConfig, ok := p.config["rules"]
	if !ok {
		return nil
	}

	rules, ok := rulesConfig.([]interface{})
	if !ok {
		return nil
	}

	// Check each rule to see if it matches
	for _, ruleConfig := range rules {
		ruleMap, ok := ruleConfig.(map[string]interface{})
		if !ok {
			continue
		}

		rule := p.parseProxyRule(ruleMap)
		if rule != nil && p.ruleMatches(rule, conn) {
			return rule
		}
	}

	return nil
}

func (p *SOCKS5ProxyPlugin) parseProxyRule(ruleMap map[string]interface{}) *ProxyRule {
	rule := &ProxyRule{}

	if zone, ok := ruleMap["zone"].(string); ok {
		rule.Zone = zone
	}
	if recipient, ok := ruleMap["recipient"].(string); ok {
		rule.Recipient = recipient
	}
	if host, ok := ruleMap["host"].(string); ok {
		rule.Host = host
	}
	if proxyHost, ok := ruleMap["proxyHost"].(string); ok {
		rule.ProxyHost = proxyHost
	}
	if proxyPort, ok := ruleMap["proxyPort"].(float64); ok {
		rule.ProxyPort = int(proxyPort)
	}
	if username, ok := ruleMap["username"].(string); ok {
		rule.Username = username
	}
	if password, ok := ruleMap["password"].(string); ok {
		rule.Password = password
	}
	if enabled, ok := ruleMap["enabled"].(bool); ok {
		rule.Enabled = enabled
	}

	// Rule must have at least proxy host and port
	if rule.ProxyHost == "" || rule.ProxyPort == 0 {
		return nil
	}

	return rule
}

func (p *SOCKS5ProxyPlugin) ruleMatches(rule *ProxyRule, conn *plugin.ConnectionData) bool {
	// Check zone match
	if rule.Zone != "" && rule.Zone != conn.Zone {
		return false
	}

	// Check recipient match (could be enhanced with pattern matching)
	if rule.Recipient != "" && rule.Recipient != conn.Recipient {
		return false
	}

	// Check host match (could be enhanced with pattern matching)
	if rule.Host != "" && rule.Host != conn.TargetHost {
		return false
	}

	return true
}