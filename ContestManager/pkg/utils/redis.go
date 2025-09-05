package utils

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
}

func NewRedisClient(client *redis.Client) *RedisClient {
	return &RedisClient{client: client}
}
func (r *RedisClient) PublishVerdictUpdate(ctx context.Context, contestID string, update interface{}) error {
	channel := fmt.Sprintf("verdicts:%s", contestID)

	data, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("failed to marshal update: %w", err)
	}

	return r.client.Publish(ctx, channel, data).Err()
}

func (r *RedisClient) SubscribeToVerdictUpdates(ctx context.Context, contestID string) *redis.PubSub {
	channel := fmt.Sprintf("verdicts:%s", contestID)
	return r.client.Subscribe(ctx, channel)
}

func (r *RedisClient) PublishLeaderboardUpdate(ctx context.Context, contestID string, update interface{}) error {
	channel := fmt.Sprintf("leaderboard:%s", contestID)

	data, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("failed to marshal leaderboard update: %w", err)
	}

	return r.client.Publish(ctx, channel, data).Err()
}

func (r *RedisClient) SubscribeToLeaderboardUpdates(ctx context.Context, contestID string) *redis.PubSub {
	channel := fmt.Sprintf("leaderboard:%s", contestID)
	return r.client.Subscribe(ctx, channel)
}
