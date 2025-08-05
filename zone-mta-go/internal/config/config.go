package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// LoadMultipleFiles loads and merges multiple YAML configuration files
// Files are loaded in alphabetical order, with later files overriding earlier ones
func LoadMultipleFiles(configPaths []string) (*Config, error) {
	if len(configPaths) == 0 {
		return nil, fmt.Errorf("no configuration files provided")
	}

	// Load first file as base
	config, err := Load(configPaths[0])
	if err != nil {
		return nil, fmt.Errorf("failed to load base config %s: %w", configPaths[0], err)
	}

	// Merge additional files
	for _, configPath := range configPaths[1:] {
		if err := mergeConfigFile(config, configPath); err != nil {
			return nil, fmt.Errorf("failed to merge config %s: %w", configPath, err)
		}
	}

	return config, nil
}

// LoadFromDirWithOverrides loads all YAML files from a directory and merges them
// Files are loaded in this order:
// 1. default.yaml (base configuration)
// 2. environment-specific files (e.g., production.yaml, development.yaml)
// 3. local overrides (e.g., local.yaml, override.yaml)
// 4. any other .yaml/.yml files in alphabetical order
func LoadFromDirWithOverrides(configDir string) (*Config, error) {
	// Check if directory exists
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration directory not found: %s", configDir)
	}

	// Find all YAML files
	files, err := findYamlFiles(configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find YAML files: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no YAML configuration files found in directory: %s", configDir)
	}

	// Sort files by priority and then alphabetically
	sortedFiles := sortConfigFiles(files)

	return LoadMultipleFiles(sortedFiles)
}

// findYamlFiles finds all YAML files in a directory
func findYamlFiles(dir string) ([]string, error) {
	var files []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check for YAML extensions
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// sortConfigFiles sorts configuration files by priority
func sortConfigFiles(files []string) []string {
	// Define priority order
	priorityOrder := map[string]int{
		"default.yaml":     1,
		"default.yml":      1,
		"base.yaml":        2,
		"base.yml":         2,
		"production.yaml":  10,
		"production.yml":   10,
		"staging.yaml":     10,
		"staging.yml":      10,
		"development.yaml": 10,
		"development.yml":  10,
		"dev.yaml":         10,
		"dev.yml":          10,
		"test.yaml":        10,
		"test.yml":         10,
		"local.yaml":       20,
		"local.yml":        20,
		"override.yaml":    30,
		"override.yml":     30,
		"secrets.yaml":     40,
		"secrets.yml":      40,
	}

	// Sort files
	sort.Slice(files, func(i, j int) bool {
		nameI := filepath.Base(files[i])
		nameJ := filepath.Base(files[j])

		priorityI, hasI := priorityOrder[nameI]
		priorityJ, hasJ := priorityOrder[nameJ]

		// If both have priorities, sort by priority
		if hasI && hasJ {
			return priorityI < priorityJ
		}

		// If only one has priority, it comes first
		if hasI {
			return true
		}
		if hasJ {
			return false
		}

		// If neither has priority, sort alphabetically
		return nameI < nameJ
	})

	return files
}

// mergeConfigFile merges a configuration file into an existing config
func mergeConfigFile(baseConfig *Config, configPath string) error {
	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("configuration file not found: %s", configPath)
	}

	// Read the file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read configuration file: %w", err)
	}

	// Parse YAML into a temporary config
	var overrideConfig Config
	if err := yaml.Unmarshal(data, &overrideConfig); err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}

	// Merge configurations
	mergeConfigs(baseConfig, &overrideConfig)

	return nil
}

// MergeConfigs merges override config into base config (exported version)
func MergeConfigs(base *Config, override *Config) {
	mergeConfigs(base, override)
}

// mergeConfigs merges override config into base config
func mergeConfigs(base *Config, override *Config) {
	// Merge app config
	if override.App.Name != "" {
		base.App.Name = override.App.Name
	}
	if override.App.Ident != "" {
		base.App.Ident = override.App.Ident
	}
	if override.App.User != "" {
		base.App.User = override.App.User
	}
	if override.App.Group != "" {
		base.App.Group = override.App.Group
	}

	// Merge database config
	if override.Databases.MongoDB.URI != "" {
		base.Databases.MongoDB.URI = override.Databases.MongoDB.URI
	}
	if override.Databases.MongoDB.Database != "" {
		base.Databases.MongoDB.Database = override.Databases.MongoDB.Database
	}
	if override.Databases.MongoDB.Timeout != 0 {
		base.Databases.MongoDB.Timeout = override.Databases.MongoDB.Timeout
	}
	if override.Databases.Redis.Host != "" {
		base.Databases.Redis.Host = override.Databases.Redis.Host
	}
	if override.Databases.Redis.Port != 0 {
		base.Databases.Redis.Port = override.Databases.Redis.Port
	}
	if override.Databases.Redis.DB != 0 {
		base.Databases.Redis.DB = override.Databases.Redis.DB
	}
	if override.Databases.Redis.Timeout != 0 {
		base.Databases.Redis.Timeout = override.Databases.Redis.Timeout
	}

	// Merge queue config
	if override.Queue.InstanceID != "" {
		base.Queue.InstanceID = override.Queue.InstanceID
	}
	if override.Queue.Collection != "" {
		base.Queue.Collection = override.Queue.Collection
	}
	if override.Queue.GridFSCollection != "" {
		base.Queue.GridFSCollection = override.Queue.GridFSCollection
	}
	if override.Queue.DefaultZone != "" {
		base.Queue.DefaultZone = override.Queue.DefaultZone
	}
	if override.Queue.MaxQueueTime != 0 {
		base.Queue.MaxQueueTime = override.Queue.MaxQueueTime
	}
	// Override boolean values directly
	base.Queue.DisableGC = override.Queue.DisableGC
	base.Queue.LogQueuePolling = override.Queue.LogQueuePolling

	// Merge SMTP interfaces (replace entirely if provided)
	if len(override.SMTP.Interfaces) > 0 {
		if base.SMTP.Interfaces == nil {
			base.SMTP.Interfaces = make(map[string]SMTPInterfaceConfig)
		}
		for name, config := range override.SMTP.Interfaces {
			base.SMTP.Interfaces[name] = config
		}
	}

	// Merge API config
	if override.API.Port != 0 {
		base.API.Port = override.API.Port
	}
	if override.API.Host != "" {
		base.API.Host = override.API.Host
	}
	if override.API.MaxRecipients != 0 {
		base.API.MaxRecipients = override.API.MaxRecipients
	}
	// Override boolean values directly
	base.API.Enabled = override.API.Enabled

	// Merge logging config
	if override.Logging.Level != "" {
		base.Logging.Level = override.Logging.Level
	}
	if override.Logging.Format != "" {
		base.Logging.Format = override.Logging.Format
	}

	// Merge metrics config
	if override.Metrics.Port != 0 {
		base.Metrics.Port = override.Metrics.Port
	}
	if override.Metrics.Path != "" {
		base.Metrics.Path = override.Metrics.Path
	}
	// Override boolean values directly
	base.Metrics.Enabled = override.Metrics.Enabled

	// Merge plugins (replace entirely if provided)
	if len(override.Plugins) > 0 {
		if base.Plugins == nil {
			base.Plugins = make(map[string]interface{})
		}
		for name, config := range override.Plugins {
			base.Plugins[name] = config
		}
	}

	// Merge sending zones (replace entirely if provided)
	if len(override.SendingZones) > 0 {
		if base.SendingZones == nil {
			base.SendingZones = make(map[string]SendingZoneConfig)
		}
		for name, config := range override.SendingZones {
			base.SendingZones[name] = config
		}
	}

	// Merge performance config
	if override.Performance.MaxConnections != 0 {
		base.Performance.MaxConnections = override.Performance.MaxConnections
	}
	if override.Performance.WorkerCount != 0 {
		base.Performance.WorkerCount = override.Performance.WorkerCount
	}
	if override.Performance.BufferSize != 0 {
		base.Performance.BufferSize = override.Performance.BufferSize
	}
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
