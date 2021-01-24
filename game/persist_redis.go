package game

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
	"github.com/golang/protobuf/proto"
)

type RedisGameStateTracker struct {
	rdclient *redis.Client
}

type RedisHandStateTracker struct {
	rdclient *redis.Client
}

func NewRedisGameStateTracker(redisURL string, redisPW string, redisDB int) *RedisGameStateTracker {
	rdclient := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPW,
		DB:       redisDB,
	})
	return &RedisGameStateTracker{
		rdclient: rdclient,
	}
}

func NewRedisHandStateTracker(redisURL string, redisPW string, redisDB int) *RedisHandStateTracker {
	rdclient := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPW,
		DB:       redisDB,
	})
	return &RedisHandStateTracker{
		rdclient: rdclient,
	}
}

func (r *RedisGameStateTracker) Load(clubID uint32, gameID uint64) (*GameState, error) {
	key := fmt.Sprintf("%d", gameID)
	gameStateBytes, err := r.rdclient.Get(context.Background(), key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("Game state for Club: %d, Game: %d is not found", clubID, gameID)
	} else if err != nil {
		return nil, err
	}
	gameState := &GameState{}
	err = proto.Unmarshal([]byte(gameStateBytes), gameState)
	if err != nil {
		return nil, err
	}
	return gameState, nil
}

func (r *RedisGameStateTracker) Save(clubID uint32, gameID uint64, state *GameState) error {
	key := fmt.Sprintf("%d", gameID)
	stateInBytes, err := proto.Marshal(state)
	if err != nil {
		return err
	}
	err = r.rdclient.Set(context.Background(), key, stateInBytes, 0).Err()
	return err
}

func (r *RedisGameStateTracker) Remove(clubID uint32, gameID uint64) error {
	key := fmt.Sprintf("%d", gameID)
	err := r.rdclient.Del(context.Background(), key).Err()
	return err
}

func (r *RedisGameStateTracker) NextGameId(clubID uint32) (uint64, error) {
	key := fmt.Sprintf("club_%d_nextgameid", clubID)
	result, err := r.rdclient.Incr(context.Background(), key).Result()
	if err != nil {
		return 0, err
	}
	return uint64(result), nil
}

func (r *RedisHandStateTracker) Load(clubID uint32, gameID uint64, handID uint32) (*HandState, error) {
	key := fmt.Sprintf("%d|%d", gameID, handID)
	handStateBytes, err := r.rdclient.Get(context.Background(), key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("Hand state for Club: %d, Game: %d, Hand: %d is not found", clubID, gameID, handID)
	} else if err != nil {
		return nil, err
	}
	handState := &HandState{}
	err = proto.Unmarshal([]byte(handStateBytes), handState)
	if err != nil {
		return nil, err
	}
	return handState, nil
}

func (r *RedisHandStateTracker) Save(clubID uint32, gameID uint64, handID uint32, state *HandState) error {
	key := fmt.Sprintf("%d|%d", gameID, handID)
	stateInBytes, err := proto.Marshal(state)
	if err != nil {
		return err
	}
	err = r.rdclient.Set(context.Background(), key, stateInBytes, 0).Err()
	return err
}

func (r *RedisHandStateTracker) Remove(clubID uint32, gameID uint64, handID uint32) error {
	key := fmt.Sprintf("%d|%d", gameID, handID)
	err := r.rdclient.Del(context.Background(), key).Err()
	return err
}
