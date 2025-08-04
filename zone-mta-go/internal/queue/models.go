package queue

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// QueuedMessage represents a message in the mail queue
type QueuedMessage struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	MessageID  string             `bson:"id" json:"messageId"`          // Unique message identifier
	Interface  string             `bson:"interface" json:"interface"`   // Interface that received the message (feeder, api, etc.)
	Zone       string             `bson:"zone" json:"zone"`             // Sending zone
	Origin     string             `bson:"origin" json:"origin"`         // Origin IP address
	OriginHost string             `bson:"originHost" json:"originHost"` // Origin hostname

	// Envelope information
	From       string   `bson:"from" json:"from"`             // Envelope sender
	Recipients []string `bson:"recipients" json:"recipients"` // Envelope recipients

	// Message metadata
	Subject     string            `bson:"subject,omitempty" json:"subject,omitempty"`
	MessageSize int64             `bson:"messageSize" json:"messageSize"`
	Headers     map[string]string `bson:"headers,omitempty" json:"headers,omitempty"`

	// Queue management
	Created          time.Time `bson:"created" json:"created"`
	NextAttempt      time.Time `bson:"nextAttempt" json:"nextAttempt"`
	DeliveryAttempts int       `bson:"deliveryAttempts" json:"deliveryAttempts"`

	// Processing state
	Status      MessageStatus `bson:"status" json:"status"`
	Locked      bool          `bson:"locked,omitempty" json:"locked,omitempty"`
	LockedUntil time.Time     `bson:"lockedUntil,omitempty" json:"lockedUntil,omitempty"`
	WorkerID    string        `bson:"workerId,omitempty" json:"workerId,omitempty"`

	// Delivery tracking
	LastError    string            `bson:"lastError,omitempty" json:"lastError,omitempty"`
	DeliveryLog  []DeliveryAttempt `bson:"deliveryLog,omitempty" json:"deliveryLog,omitempty"`
	BounceReason string            `bson:"bounceReason,omitempty" json:"bounceReason,omitempty"`

	// Additional metadata
	Tags     []string               `bson:"tags,omitempty" json:"tags,omitempty"`
	Metadata map[string]interface{} `bson:"metadata,omitempty" json:"metadata,omitempty"`

	// DKIM information
	DKIMKeys []DKIMKey `bson:"dkimKeys,omitempty" json:"dkimKeys,omitempty"`

	// Performance tracking
	QueueTime    time.Duration `bson:"queueTime,omitempty" json:"queueTime,omitempty"`
	DeliveryTime time.Duration `bson:"deliveryTime,omitempty" json:"deliveryTime,omitempty"`
}

// MessageStatus represents the status of a message in the queue
type MessageStatus string

const (
	StatusQueued    MessageStatus = "queued"    // Message is waiting to be sent
	StatusSending   MessageStatus = "sending"   // Message is currently being sent
	StatusDelivered MessageStatus = "delivered" // Message was successfully delivered
	StatusBounced   MessageStatus = "bounced"   // Message bounced (permanent failure)
	StatusDeferred  MessageStatus = "deferred"  // Message delivery was deferred (temporary failure)
	StatusRejected  MessageStatus = "rejected"  // Message was rejected by policy
)

// DeliveryAttempt represents a single delivery attempt
type DeliveryAttempt struct {
	Timestamp  time.Time `bson:"timestamp" json:"timestamp"`
	Recipient  string    `bson:"recipient" json:"recipient"`
	Status     string    `bson:"status" json:"status"`     // "delivered", "bounced", "deferred"
	Response   string    `bson:"response" json:"response"` // SMTP response
	MX         string    `bson:"mx,omitempty" json:"mx,omitempty"`
	IP         string    `bson:"ip,omitempty" json:"ip,omitempty"`
	Duration   int64     `bson:"duration" json:"duration"` // Delivery duration in milliseconds
	RetryCount int       `bson:"retryCount" json:"retryCount"`
}

// DKIMKey represents DKIM signing information
type DKIMKey struct {
	Domain     string `bson:"domain" json:"domain"`
	Selector   string `bson:"selector" json:"selector"`
	Algorithm  string `bson:"algorithm" json:"algorithm"`
	PrivateKey string `bson:"privateKey" json:"privateKey"`
}

// QueueStats represents queue statistics
type QueueStats struct {
	TotalMessages     int64            `json:"totalMessages"`
	QueuedMessages    int64            `json:"queuedMessages"`
	SendingMessages   int64            `json:"sendingMessages"`
	DeliveredMessages int64            `json:"deliveredMessages"`
	BouncedMessages   int64            `json:"bouncedMessages"`
	DeferredMessages  int64            `json:"deferredMessages"`
	ZoneStats         map[string]int64 `json:"zoneStats"`
	OldestMessage     *time.Time       `json:"oldestMessage,omitempty"`
}

// QueueFilter represents filters for queue queries
type QueueFilter struct {
	Zone          string        `json:"zone,omitempty"`
	Status        MessageStatus `json:"status,omitempty"`
	Interface     string        `json:"interface,omitempty"`
	Recipient     string        `json:"recipient,omitempty"`
	From          string        `json:"from,omitempty"`
	CreatedAfter  *time.Time    `json:"createdAfter,omitempty"`
	CreatedBefore *time.Time    `json:"createdBefore,omitempty"`
	Tags          []string      `json:"tags,omitempty"`
	Limit         int           `json:"limit,omitempty"`
	Skip          int           `json:"skip,omitempty"`
}

// MessageInfo represents basic message information for listing
type MessageInfo struct {
	ID               string        `json:"id"`
	MessageID        string        `json:"messageId"`
	Zone             string        `json:"zone"`
	From             string        `json:"from"`
	Recipients       []string      `json:"recipients"`
	Subject          string        `json:"subject,omitempty"`
	Status           MessageStatus `json:"status"`
	Created          time.Time     `json:"created"`
	NextAttempt      time.Time     `json:"nextAttempt"`
	DeliveryAttempts int           `json:"deliveryAttempts"`
	MessageSize      int64         `json:"messageSize"`
	LastError        string        `json:"lastError,omitempty"`
}
