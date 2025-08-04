package db

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/zone-eu/zone-mta-go/internal/config"
)

// MongoDB represents a MongoDB connection
type MongoDB struct {
	client   *mongo.Client
	database *mongo.Database
	gridFS   *gridfs.Bucket
	config   config.MongoDBConfig
}

// NewMongoDB creates a new MongoDB connection
func NewMongoDB(cfg config.MongoDBConfig) (*MongoDB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// Create client options
	clientOptions := options.Client().ApplyURI(cfg.URI)

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping to verify connection
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	// Get database
	database := client.Database(cfg.Database)

	// Create GridFS bucket for mail storage
	bucket, err := gridfs.NewBucket(database, options.GridFSBucket().SetName("mail"))
	if err != nil {
		return nil, fmt.Errorf("failed to create GridFS bucket: %w", err)
	}

	return &MongoDB{
		client:   client,
		database: database,
		gridFS:   bucket,
		config:   cfg,
	}, nil
}

// Close closes the MongoDB connection
func (m *MongoDB) Close(ctx context.Context) error {
	return m.client.Disconnect(ctx)
}

// Database returns the MongoDB database instance
func (m *MongoDB) Database() *mongo.Database {
	return m.database
}

// GridFS returns the GridFS bucket for mail storage
func (m *MongoDB) GridFS() *gridfs.Bucket {
	return m.gridFS
}

// Collection returns a collection by name
func (m *MongoDB) Collection(name string) *mongo.Collection {
	return m.database.Collection(name)
}

// Ping checks the connection to MongoDB
func (m *MongoDB) Ping(ctx context.Context) error {
	return m.client.Ping(ctx, readpref.Primary())
}

// CreateIndexes creates necessary indexes for ZoneMTA collections
func (m *MongoDB) CreateIndexes(ctx context.Context) error {
	// Create indexes for the queue collection
	queueCollection := m.Collection("zone-queue")

	// Index for zone and next delivery time
	_, err := queueCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: map[string]interface{}{
			"zone":        1,
			"nextAttempt": 1,
		},
		Options: options.Index().SetName("zone_nextAttempt"),
	})
	if err != nil {
		return fmt.Errorf("failed to create zone_nextAttempt index: %w", err)
	}

	// Index for message ID
	_, err = queueCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: map[string]interface{}{
			"id": 1,
		},
		Options: options.Index().SetName("id").SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("failed to create id index: %w", err)
	}

	// Index for created timestamp (for cleanup)
	_, err = queueCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: map[string]interface{}{
			"created": 1,
		},
		Options: options.Index().SetName("created"),
	})
	if err != nil {
		return fmt.Errorf("failed to create created index: %w", err)
	}

	// Index for delivery attempts
	_, err = queueCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: map[string]interface{}{
			"deliveryAttempts": 1,
		},
		Options: options.Index().SetName("deliveryAttempts"),
	})
	if err != nil {
		return fmt.Errorf("failed to create deliveryAttempts index: %w", err)
	}

	return nil
}

// HealthCheck performs a health check on the MongoDB connection
func (m *MongoDB) HealthCheck(ctx context.Context) error {
	// Use a short timeout for health checks
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return m.Ping(healthCtx)
}
