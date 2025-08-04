package db

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/zone-eu/zone-mta-go/internal/config"
)

// Redis represents a Redis connection
type Redis struct {
	client *redis.Client
	config config.RedisConfig
}

// NewRedis creates a new Redis connection
func NewRedis(cfg config.RedisConfig) (*Redis, error) {
	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		DB:           cfg.DB,
		DialTimeout:  cfg.Timeout,
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Redis{
		client: client,
		config: cfg,
	}, nil
}

// Close closes the Redis connection
func (r *Redis) Close() error {
	return r.client.Close()
}

// Client returns the Redis client instance
func (r *Redis) Client() *redis.Client {
	return r.client
}

// Ping checks the connection to Redis
func (r *Redis) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Lock creates a distributed lock with expiration
func (r *Redis) Lock(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	result := r.client.SetNX(ctx, key, "locked", expiration)
	return result.Val(), result.Err()
}

// Unlock removes a distributed lock
func (r *Redis) Unlock(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// Increment increments a counter and returns the new value
func (r *Redis) Increment(ctx context.Context, key string) (int64, error) {
	result := r.client.Incr(ctx, key)
	return result.Val(), result.Err()
}

// IncrementBy increments a counter by a specific amount
func (r *Redis) IncrementBy(ctx context.Context, key string, value int64) (int64, error) {
	result := r.client.IncrBy(ctx, key, value)
	return result.Val(), result.Err()
}

// IncrementWithExpire increments a counter and sets expiration if it's a new key
func (r *Redis) IncrementWithExpire(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	pipe := r.client.TxPipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, expiration)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}

	return incrCmd.Val(), nil
}

// Get retrieves a value by key
func (r *Redis) Get(ctx context.Context, key string) (string, error) {
	result := r.client.Get(ctx, key)
	return result.Val(), result.Err()
}

// Set stores a value with expiration
func (r *Redis) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return r.client.Set(ctx, key, value, expiration).Err()
}

// Delete removes keys
func (r *Redis) Delete(ctx context.Context, keys ...string) error {
	return r.client.Del(ctx, keys...).Err()
}

// Exists checks if keys exist
func (r *Redis) Exists(ctx context.Context, keys ...string) (int64, error) {
	result := r.client.Exists(ctx, keys...)
	return result.Val(), result.Err()
}

// SetAdd adds members to a set
func (r *Redis) SetAdd(ctx context.Context, key string, members ...interface{}) (int64, error) {
	result := r.client.SAdd(ctx, key, members...)
	return result.Val(), result.Err()
}

// SetRemove removes members from a set
func (r *Redis) SetRemove(ctx context.Context, key string, members ...interface{}) (int64, error) {
	result := r.client.SRem(ctx, key, members...)
	return result.Val(), result.Err()
}

// SetMembers returns all members of a set
func (r *Redis) SetMembers(ctx context.Context, key string) ([]string, error) {
	result := r.client.SMembers(ctx, key)
	return result.Val(), result.Err()
}

// SetIsMember checks if a value is a member of a set
func (r *Redis) SetIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	result := r.client.SIsMember(ctx, key, member)
	return result.Val(), result.Err()
}

// HashSet sets fields in a hash
func (r *Redis) HashSet(ctx context.Context, key string, values ...interface{}) error {
	return r.client.HSet(ctx, key, values...).Err()
}

// HashGet gets a field from a hash
func (r *Redis) HashGet(ctx context.Context, key, field string) (string, error) {
	result := r.client.HGet(ctx, key, field)
	return result.Val(), result.Err()
}

// HashGetAll gets all fields from a hash
func (r *Redis) HashGetAll(ctx context.Context, key string) (map[string]string, error) {
	result := r.client.HGetAll(ctx, key)
	return result.Val(), result.Err()
}

// HashDelete deletes fields from a hash
func (r *Redis) HashDelete(ctx context.Context, key string, fields ...string) (int64, error) {
	result := r.client.HDel(ctx, key, fields...)
	return result.Val(), result.Err()
}

// ListPush pushes values to the head of a list
func (r *Redis) ListPush(ctx context.Context, key string, values ...interface{}) (int64, error) {
	result := r.client.LPush(ctx, key, values...)
	return result.Val(), result.Err()
}

// ListPop pops a value from the head of a list
func (r *Redis) ListPop(ctx context.Context, key string) (string, error) {
	result := r.client.LPop(ctx, key)
	return result.Val(), result.Err()
}

// ListLength returns the length of a list
func (r *Redis) ListLength(ctx context.Context, key string) (int64, error) {
	result := r.client.LLen(ctx, key)
	return result.Val(), result.Err()
}

// HealthCheck performs a health check on the Redis connection
func (r *Redis) HealthCheck(ctx context.Context) error {
	// Use a short timeout for health checks
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return r.Ping(healthCtx)
}
