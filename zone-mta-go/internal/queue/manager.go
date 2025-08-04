package queue

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/zone-eu/zone-mta-go/internal/config"
	"github.com/zone-eu/zone-mta-go/internal/db"
)

// Manager handles mail queue operations
type Manager struct {
	mongodb    *db.MongoDB
	redis      *db.Redis
	collection *mongo.Collection
	gridFS     *gridfs.Bucket
	config     config.QueueConfig
	logger     *slog.Logger
}

// NewManager creates a new queue manager
func NewManager(dbManager *db.Manager, cfg config.QueueConfig, logger *slog.Logger) *Manager {
	return &Manager{
		mongodb:    dbManager.MongoDB,
		redis:      dbManager.Redis,
		collection: dbManager.MongoDB.Collection(cfg.Collection),
		gridFS:     dbManager.MongoDB.GridFS(),
		config:     cfg,
		logger:     logger,
	}
}

// Store stores a message in the queue and its content in GridFS
func (m *Manager) Store(ctx context.Context, msg *QueuedMessage, content io.Reader) error {
	// Generate a unique message ID if not provided
	if msg.MessageID == "" {
		msg.MessageID = primitive.NewObjectID().Hex()
	}

	// Set creation time
	if msg.Created.IsZero() {
		msg.Created = time.Now()
	}

	// Set default status
	if msg.Status == "" {
		msg.Status = StatusQueued
	}

	// Set default zone
	if msg.Zone == "" {
		msg.Zone = m.config.DefaultZone
	}

	// Set next attempt time
	if msg.NextAttempt.IsZero() {
		msg.NextAttempt = time.Now()
	}

	// Store message content in GridFS
	uploadStream, err := m.gridFS.OpenUploadStreamWithID(msg.MessageID, fmt.Sprintf("message-%s", msg.MessageID))
	if err != nil {
		return fmt.Errorf("failed to create GridFS upload stream: %w", err)
	}
	defer uploadStream.Close()

	// Copy content to GridFS
	size, err := io.Copy(uploadStream, content)
	if err != nil {
		return fmt.Errorf("failed to store message content: %w", err)
	}

	msg.MessageSize = size

	// Store message metadata in MongoDB
	_, err = m.collection.InsertOne(ctx, msg)
	if err != nil {
		// Clean up GridFS file on failure
		m.gridFS.Delete(msg.MessageID)
		return fmt.Errorf("failed to store message metadata: %w", err)
	}

	m.logger.Info("Message queued",
		"messageId", msg.MessageID,
		"zone", msg.Zone,
		"from", msg.From,
		"recipients", len(msg.Recipients),
		"size", size)

	return nil
}

// Get retrieves a message by ID
func (m *Manager) Get(ctx context.Context, messageID string) (*QueuedMessage, error) {
	var msg QueuedMessage
	err := m.collection.FindOne(ctx, bson.M{"id": messageID}).Decode(&msg)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("message not found: %s", messageID)
		}
		return nil, fmt.Errorf("failed to retrieve message: %w", err)
	}

	return &msg, nil
}

// GetContent retrieves message content from GridFS
func (m *Manager) GetContent(ctx context.Context, messageID string) (io.ReadCloser, error) {
	downloadStream, err := m.gridFS.OpenDownloadStream(messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get message content: %w", err)
	}

	return downloadStream, nil
}

// GetNext retrieves the next message to be delivered for a specific zone
func (m *Manager) GetNext(ctx context.Context, zone string, workerID string) (*QueuedMessage, error) {
	now := time.Now()
	lockTimeout := 10 * time.Minute // Lock timeout to prevent stuck messages

	// Find and lock a message atomically
	filter := bson.M{
		"zone":        zone,
		"status":      bson.M{"$in": []MessageStatus{StatusQueued, StatusDeferred}},
		"nextAttempt": bson.M{"$lte": now},
		"$or": []bson.M{
			{"locked": bson.M{"$ne": true}},
			{"lockedUntil": bson.M{"$lte": now}},
		},
	}

	update := bson.M{
		"$set": bson.M{
			"locked":      true,
			"lockedUntil": now.Add(lockTimeout),
			"workerId":    workerID,
			"status":      StatusSending,
		},
	}

	opts := options.FindOneAndUpdate().SetSort(bson.M{"nextAttempt": 1}).SetReturnDocument(options.After)

	var msg QueuedMessage
	err := m.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&msg)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No messages available
		}
		return nil, fmt.Errorf("failed to get next message: %w", err)
	}

	m.logger.Debug("Retrieved message for delivery",
		"messageId", msg.MessageID,
		"zone", zone,
		"workerId", workerID,
		"attempts", msg.DeliveryAttempts)

	return &msg, nil
}

// UpdateStatus updates the status of a message
func (m *Manager) UpdateStatus(ctx context.Context, messageID string, status MessageStatus, errorMsg string) error {
	update := bson.M{
		"$set": bson.M{
			"status":   status,
			"locked":   false,
			"workerId": "",
		},
		"$unset": bson.M{
			"lockedUntil": "",
		},
	}

	if errorMsg != "" {
		update["$set"].(bson.M)["lastError"] = errorMsg
	}

	_, err := m.collection.UpdateOne(ctx, bson.M{"id": messageID}, update)
	if err != nil {
		return fmt.Errorf("failed to update message status: %w", err)
	}

	return nil
}

// RecordDeliveryAttempt records a delivery attempt
func (m *Manager) RecordDeliveryAttempt(ctx context.Context, messageID string, attempt DeliveryAttempt) error {
	now := time.Now()

	update := bson.M{
		"$push": bson.M{
			"deliveryLog": attempt,
		},
		"$inc": bson.M{
			"deliveryAttempts": 1,
		},
		"$set": bson.M{
			"locked":   false,
			"workerId": "",
		},
		"$unset": bson.M{
			"lockedUntil": "",
		},
	}

	// Set next attempt time based on delivery result
	if attempt.Status == "delivered" {
		update["$set"].(bson.M)["status"] = StatusDelivered
		update["$set"].(bson.M)["deliveryTime"] = time.Since(attempt.Timestamp)
	} else if attempt.Status == "bounced" {
		update["$set"].(bson.M)["status"] = StatusBounced
		update["$set"].(bson.M)["bounceReason"] = attempt.Response
	} else { // deferred
		// Calculate next retry time with exponential backoff
		retryDelay := m.calculateRetryDelay(attempt.RetryCount)
		update["$set"].(bson.M)["status"] = StatusDeferred
		update["$set"].(bson.M)["nextAttempt"] = now.Add(retryDelay)
		update["$set"].(bson.M)["lastError"] = attempt.Response
	}

	_, err := m.collection.UpdateOne(ctx, bson.M{"id": messageID}, update)
	if err != nil {
		return fmt.Errorf("failed to record delivery attempt: %w", err)
	}

	m.logger.Info("Delivery attempt recorded",
		"messageId", messageID,
		"recipient", attempt.Recipient,
		"status", attempt.Status,
		"retryCount", attempt.RetryCount)

	return nil
}

// calculateRetryDelay calculates the retry delay with exponential backoff
func (m *Manager) calculateRetryDelay(retryCount int) time.Duration {
	// Exponential backoff: 1min, 5min, 15min, 30min, 1h, 2h, 4h, then 4h intervals
	delays := []time.Duration{
		1 * time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
		1 * time.Hour,
		2 * time.Hour,
		4 * time.Hour,
	}

	if retryCount < len(delays) {
		return delays[retryCount]
	}

	// After all predefined delays, retry every 4 hours
	return 4 * time.Hour
}

// Delete removes a message from the queue and GridFS
func (m *Manager) Delete(ctx context.Context, messageID string) error {
	// Delete from queue collection
	_, err := m.collection.DeleteOne(ctx, bson.M{"id": messageID})
	if err != nil {
		return fmt.Errorf("failed to delete message from queue: %w", err)
	}

	// Delete from GridFS
	err = m.gridFS.Delete(messageID)
	if err != nil {
		m.logger.Warn("Failed to delete GridFS file", "messageId", messageID, "error", err)
		// Don't return error as the main queue record is already deleted
	}

	m.logger.Info("Message deleted", "messageId", messageID)
	return nil
}

// List retrieves messages based on filter criteria
func (m *Manager) List(ctx context.Context, filter QueueFilter) ([]MessageInfo, error) {
	// Build MongoDB filter
	mongoFilter := bson.M{}

	if filter.Zone != "" {
		mongoFilter["zone"] = filter.Zone
	}
	if filter.Status != "" {
		mongoFilter["status"] = filter.Status
	}
	if filter.Interface != "" {
		mongoFilter["interface"] = filter.Interface
	}
	if filter.From != "" {
		mongoFilter["from"] = bson.M{"$regex": primitive.Regex{Pattern: filter.From, Options: "i"}}
	}
	if filter.Recipient != "" {
		mongoFilter["recipients"] = bson.M{"$regex": primitive.Regex{Pattern: filter.Recipient, Options: "i"}}
	}
	if filter.CreatedAfter != nil || filter.CreatedBefore != nil {
		timeFilter := bson.M{}
		if filter.CreatedAfter != nil {
			timeFilter["$gte"] = *filter.CreatedAfter
		}
		if filter.CreatedBefore != nil {
			timeFilter["$lte"] = *filter.CreatedBefore
		}
		mongoFilter["created"] = timeFilter
	}
	if len(filter.Tags) > 0 {
		mongoFilter["tags"] = bson.M{"$in": filter.Tags}
	}

	// Set default limit
	limit := filter.Limit
	if limit == 0 {
		limit = 100
	}

	// Query options
	opts := options.Find().
		SetLimit(int64(limit)).
		SetSkip(int64(filter.Skip)).
		SetSort(bson.M{"created": -1})

	// Execute query
	cursor, err := m.collection.Find(ctx, mongoFilter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer cursor.Close(ctx)

	// Convert to MessageInfo structs
	var messages []MessageInfo
	for cursor.Next(ctx) {
		var msg QueuedMessage
		if err := cursor.Decode(&msg); err != nil {
			return nil, fmt.Errorf("failed to decode message: %w", err)
		}

		info := MessageInfo{
			ID:               msg.ID.Hex(),
			MessageID:        msg.MessageID,
			Zone:             msg.Zone,
			From:             msg.From,
			Recipients:       msg.Recipients,
			Subject:          msg.Subject,
			Status:           msg.Status,
			Created:          msg.Created,
			NextAttempt:      msg.NextAttempt,
			DeliveryAttempts: msg.DeliveryAttempts,
			MessageSize:      msg.MessageSize,
			LastError:        msg.LastError,
		}
		messages = append(messages, info)
	}

	return messages, nil
}

// GetStats returns queue statistics
func (m *Manager) GetStats(ctx context.Context) (*QueueStats, error) {
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":   "$status",
				"count": bson.M{"$sum": 1},
			},
		},
	}

	cursor, err := m.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue stats: %w", err)
	}
	defer cursor.Close(ctx)

	stats := &QueueStats{
		ZoneStats: make(map[string]int64),
	}

	for cursor.Next(ctx) {
		var result struct {
			ID    MessageStatus `bson:"_id"`
			Count int64         `bson:"count"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode stats: %w", err)
		}

		stats.TotalMessages += result.Count

		switch result.ID {
		case StatusQueued:
			stats.QueuedMessages = result.Count
		case StatusSending:
			stats.SendingMessages = result.Count
		case StatusDelivered:
			stats.DeliveredMessages = result.Count
		case StatusBounced:
			stats.BouncedMessages = result.Count
		case StatusDeferred:
			stats.DeferredMessages = result.Count
		}
	}

	// Get zone statistics
	zonePipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":   "$zone",
				"count": bson.M{"$sum": 1},
			},
		},
	}

	zoneCursor, err := m.collection.Aggregate(ctx, zonePipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to get zone stats: %w", err)
	}
	defer zoneCursor.Close(ctx)

	for zoneCursor.Next(ctx) {
		var result struct {
			Zone  string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := zoneCursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode zone stats: %w", err)
		}

		stats.ZoneStats[result.Zone] = result.Count
	}

	// Get oldest message
	var oldestMsg QueuedMessage
	opts := options.FindOne().SetSort(bson.M{"created": 1})
	err = m.collection.FindOne(ctx, bson.M{}, opts).Decode(&oldestMsg)
	if err == nil {
		stats.OldestMessage = &oldestMsg.Created
	}

	return stats, nil
}

// CleanupOldMessages removes old messages that exceed the max queue time
func (m *Manager) CleanupOldMessages(ctx context.Context) error {
	if m.config.DisableGC {
		return nil
	}

	cutoff := time.Now().Add(-m.config.MaxQueueTime)

	// Find old messages to delete
	filter := bson.M{
		"created": bson.M{"$lt": cutoff},
		"status":  bson.M{"$in": []MessageStatus{StatusDelivered, StatusBounced}},
	}

	cursor, err := m.collection.Find(ctx, filter, options.Find().SetProjection(bson.M{"id": 1}))
	if err != nil {
		return fmt.Errorf("failed to find old messages: %w", err)
	}
	defer cursor.Close(ctx)

	var deletedCount int64
	for cursor.Next(ctx) {
		var msg struct {
			MessageID string `bson:"id"`
		}
		if err := cursor.Decode(&msg); err != nil {
			continue
		}

		// Delete the message (this also removes from GridFS)
		if err := m.Delete(ctx, msg.MessageID); err != nil {
			m.logger.Warn("Failed to delete old message", "messageId", msg.MessageID, "error", err)
			continue
		}

		deletedCount++
	}

	if deletedCount > 0 {
		m.logger.Info("Cleaned up old messages", "count", deletedCount)
	}

	return nil
}

// ReleaseStaleLocks releases locks that have expired
func (m *Manager) ReleaseStaleLocks(ctx context.Context) error {
	now := time.Now()

	update := bson.M{
		"$set": bson.M{
			"locked": false,
			"status": StatusQueued,
		},
		"$unset": bson.M{
			"lockedUntil": "",
			"workerId":    "",
		},
	}

	filter := bson.M{
		"locked":      true,
		"lockedUntil": bson.M{"$lte": now},
	}

	result, err := m.collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to release stale locks: %w", err)
	}

	if result.ModifiedCount > 0 {
		m.logger.Info("Released stale locks", "count", result.ModifiedCount)
	}

	return nil
}
