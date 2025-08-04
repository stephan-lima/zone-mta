package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete ZoneMTA configuration
type Config struct {
	App          AppConfig                    `yaml:"app"`
	Databases    DatabasesConfig              `yaml:"databases"`
	Queue        QueueConfig                  `yaml:"queue"`
	SMTP         SMTPConfig                   `yaml:"smtp"`
	API          APIConfig                    `yaml:"api"`
	Logging      LoggingConfig                `yaml:"logging"`
	Metrics      MetricsConfig                `yaml:"metrics"`
	Plugins      map[string]interface{}       `yaml:"plugins"`
	SendingZones map[string]SendingZoneConfig `yaml:"sendingZones"`
	Performance  PerformanceConfig            `yaml:"performance"`
}

// AppConfig contains application-level settings
type AppConfig struct {
	Name  string `yaml:"name"`
	Ident string `yaml:"ident"`
	User  string `yaml:"user,omitempty"`
	Group string `yaml:"group,omitempty"`
}

// DatabasesConfig contains database connection settings
type DatabasesConfig struct {
	MongoDB MongoDBConfig `yaml:"mongodb"`
	Redis   RedisConfig   `yaml:"redis"`
}

// MongoDBConfig contains MongoDB connection settings
type MongoDBConfig struct {
	URI      string        `yaml:"uri"`
	Database string        `yaml:"database"`
	Timeout  time.Duration `yaml:"timeout"`
}

// RedisConfig contains Redis connection settings
type RedisConfig struct {
	Host    string        `yaml:"host"`
	Port    int           `yaml:"port"`
	DB      int           `yaml:"db"`
	Timeout time.Duration `yaml:"timeout"`
}

// QueueConfig contains mail queue settings
type QueueConfig struct {
	InstanceID       string        `yaml:"instanceId"`
	Collection       string        `yaml:"collection"`
	GridFSCollection string        `yaml:"gridfsCollection"`
	DisableGC        bool          `yaml:"disableGC"`
	DefaultZone      string        `yaml:"defaultZone"`
	MaxQueueTime     time.Duration `yaml:"maxQueueTime"`
	LogQueuePolling  bool          `yaml:"logQueuePolling"`
}

// SMTPConfig contains SMTP server configuration
type SMTPConfig struct {
	Interfaces map[string]SMTPInterfaceConfig `yaml:"interfaces"`
}

// SMTPInterfaceConfig contains settings for a single SMTP interface
type SMTPInterfaceConfig struct {
	Enabled         bool   `yaml:"enabled"`
	Port            int    `yaml:"port"`
	Host            string `yaml:"host"`
	Secure          bool   `yaml:"secure"`
	Authentication  bool   `yaml:"authentication"`
	MaxConnections  int    `yaml:"maxConnections"`
	MaxRecipients   int    `yaml:"maxRecipients"`
	DisableSTARTTLS bool   `yaml:"disableSTARTTLS"`
}

// APIConfig contains HTTP API settings
type APIConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Port          int    `yaml:"port"`
	Host          string `yaml:"host"`
	MaxRecipients int    `yaml:"maxRecipients"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// MetricsConfig contains metrics/monitoring settings
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Path    string `yaml:"path"`
}

// SendingZoneConfig contains settings for a sending zone
type SendingZoneConfig struct {
	Name            string        `yaml:"name"`
	Connections     int           `yaml:"connections"`
	ConnectionTime  time.Duration `yaml:"connectionTime"`
	Throttling      string        `yaml:"throttling"`
	PoolConnections bool          `yaml:"poolConnections"`
}

// PerformanceConfig contains performance tuning settings
type PerformanceConfig struct {
	MaxConnections int `yaml:"maxConnections"`
	WorkerCount    int `yaml:"workerCount"`
	BufferSize     int `yaml:"bufferSize"`
}

// Load loads configuration from a YAML file
func Load(configPath string) (*Config, error) {
	if configPath == "" {
		configPath = "config/default.yaml"
	}

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", configPath)
	}

	// Read the file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %w", err)
	}

	// Parse YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Set defaults
	setDefaults(&config)

	return &config, nil
}

// LoadFromDir loads configuration from a directory, looking for default.yaml or config.yaml
func LoadFromDir(configDir string) (*Config, error) {
	candidates := []string{"default.yaml", "config.yaml", "zone-mta.yaml"}

	for _, filename := range candidates {
		configPath := filepath.Join(configDir, filename)
		if _, err := os.Stat(configPath); err == nil {
			return Load(configPath)
		}
	}

	return nil, fmt.Errorf("no configuration file found in directory: %s", configDir)
}

// setDefaults sets default values for configuration options
func setDefaults(config *Config) {
	// App defaults
	if config.App.Name == "" {
		config.App.Name = "ZoneMTA-Go"
	}
	if config.App.Ident == "" {
		config.App.Ident = "zone-mta-go"
	}

	// Database defaults
	if config.Databases.MongoDB.URI == "" {
		config.Databases.MongoDB.URI = "mongodb://127.0.0.1:27017/zone-mta-go"
	}
	if config.Databases.MongoDB.Database == "" {
		config.Databases.MongoDB.Database = "zone-mta-go"
	}
	if config.Databases.MongoDB.Timeout == 0 {
		config.Databases.MongoDB.Timeout = 10 * time.Second
	}

	if config.Databases.Redis.Host == "" {
		config.Databases.Redis.Host = "127.0.0.1"
	}
	if config.Databases.Redis.Port == 0 {
		config.Databases.Redis.Port = 6379
	}
	if config.Databases.Redis.Timeout == 0 {
		config.Databases.Redis.Timeout = 10 * time.Second
	}

	// Queue defaults
	if config.Queue.InstanceID == "" {
		config.Queue.InstanceID = "default"
	}
	if config.Queue.Collection == "" {
		config.Queue.Collection = "zone-queue"
	}
	if config.Queue.GridFSCollection == "" {
		config.Queue.GridFSCollection = "mail"
	}
	if config.Queue.DefaultZone == "" {
		config.Queue.DefaultZone = "default"
	}
	if config.Queue.MaxQueueTime == 0 {
		config.Queue.MaxQueueTime = 30 * 24 * time.Hour // 30 days
	}

	// Logging defaults
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	if config.Logging.Format == "" {
		config.Logging.Format = "text"
	}

	// Performance defaults
	if config.Performance.MaxConnections == 0 {
		config.Performance.MaxConnections = 1000
	}
	if config.Performance.WorkerCount == 0 {
		config.Performance.WorkerCount = 4
	}
	if config.Performance.BufferSize == 0 {
		config.Performance.BufferSize = 4096
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate required fields
	if c.App.Name == "" {
		return fmt.Errorf("app name is required")
	}

	if c.Databases.MongoDB.URI == "" {
		return fmt.Errorf("mongodb URI is required")
	}

	if c.Queue.InstanceID == "" {
		return fmt.Errorf("queue instance ID is required")
	}

	// Validate ports
	if c.API.Enabled && (c.API.Port <= 0 || c.API.Port > 65535) {
		return fmt.Errorf("invalid API port: %d", c.API.Port)
	}

	// Validate SMTP interfaces
	for name, iface := range c.SMTP.Interfaces {
		if iface.Enabled && (iface.Port <= 0 || iface.Port > 65535) {
			return fmt.Errorf("invalid SMTP port for interface %s: %d", name, iface.Port)
		}
	}

	return nil
}
