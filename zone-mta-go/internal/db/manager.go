package db

import (
	"context"
	"fmt"
	"time"

	"github.com/zone-eu/zone-mta-go/internal/config"
)

// Manager manages database connections
type Manager struct {
	MongoDB *MongoDB
	Redis   *Redis
	config  config.DatabasesConfig
}

// NewManager creates a new database manager
func NewManager(cfg config.DatabasesConfig) (*Manager, error) {
	// Connect to MongoDB
	mongodb, err := NewMongoDB(cfg.MongoDB)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Connect to Redis
	redis, err := NewRedis(cfg.Redis)
	if err != nil {
		// Close MongoDB connection on Redis failure
		mongodb.Close(context.Background())
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Manager{
		MongoDB: mongodb,
		Redis:   redis,
		config:  cfg,
	}, nil
}

// Close closes all database connections
func (m *Manager) Close(ctx context.Context) error {
	var mongoErr, redisErr error

	// Close Redis
	if m.Redis != nil {
		redisErr = m.Redis.Close()
	}

	// Close MongoDB
	if m.MongoDB != nil {
		mongoErr = m.MongoDB.Close(ctx)
	}

	// Return any errors
	if mongoErr != nil && redisErr != nil {
		return fmt.Errorf("multiple errors: MongoDB: %v, Redis: %v", mongoErr, redisErr)
	}
	if mongoErr != nil {
		return fmt.Errorf("MongoDB error: %w", mongoErr)
	}
	if redisErr != nil {
		return fmt.Errorf("Redis error: %w", redisErr)
	}

	return nil
}

// Initialize sets up databases (creates indexes, etc.)
func (m *Manager) Initialize(ctx context.Context) error {
	// Create MongoDB indexes
	if err := m.MongoDB.CreateIndexes(ctx); err != nil {
		return fmt.Errorf("failed to create MongoDB indexes: %w", err)
	}

	return nil
}

// HealthCheck performs health checks on all database connections
func (m *Manager) HealthCheck(ctx context.Context) error {
	// Check MongoDB
	if err := m.MongoDB.HealthCheck(ctx); err != nil {
		return fmt.Errorf("MongoDB health check failed: %w", err)
	}

	// Check Redis
	if err := m.Redis.HealthCheck(ctx); err != nil {
		return fmt.Errorf("Redis health check failed: %w", err)
	}

	return nil
}

// WaitForConnections waits for database connections to be ready
func (m *Manager) WaitForConnections(ctx context.Context, maxRetries int, retryInterval time.Duration) error {
	for i := 0; i < maxRetries; i++ {
		if err := m.HealthCheck(ctx); err == nil {
			return nil
		}

		if i < maxRetries-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryInterval):
				continue
			}
		}
	}

	return fmt.Errorf("failed to connect to databases after %d retries", maxRetries)
}
