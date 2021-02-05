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

func (r *RedisGameStateTracker) Load(gameCode string) (*GameState, error) {
	key := gameCode
	gameStateBytes, err := r.rdclient.Get(context.Background(), key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("Game state for Game: %s is not found", gameCode)
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

func (r *RedisGameStateTracker) Save(gameCode string, state *GameState) error {
	stateInBytes, err := proto.Marshal(state)
	if err != nil {
		return err
	}
	err = r.rdclient.Set(context.Background(), gameCode, stateInBytes, 0).Err()
	return err
}

func (r *RedisGameStateTracker) Remove(gameCode string) error {
	err := r.rdclient.Del(context.Background(), gameCode).Err()
	return err
}

func (r *RedisHandStateTracker) Load(gameCode string, handID uint32) (*HandState, error) {
	key := fmt.Sprintf("%s|%d", gameCode, handID)
	return r.load(key, gameCode, handID)
}

func (r *RedisHandStateTracker) LoadClone(gameCode string, handID uint32) (*HandState, error) {
	key := fmt.Sprintf("%s|%d|clone", gameCode, handID)
	return r.load(key, gameCode, handID)
}

func (r *RedisHandStateTracker) load(key string, gameCode string, handID uint32) (*HandState, error) {
	handStateBytes, err := r.rdclient.Get(context.Background(), key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("Hand state for Game: %s, Hand: %d is not found", gameCode, handID)
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

func (r *RedisHandStateTracker) Save(gameCode string, handID uint32, state *HandState) error {
	key := fmt.Sprintf("%s|%d", gameCode, handID)
	return r.save(key, state)
}

func (r *RedisHandStateTracker) SaveClone(gameCode string, handID uint32, state *HandState) error {
	key := fmt.Sprintf("%s|%d|clone", gameCode, handID)
	return r.save(key, state)
}

func (r *RedisHandStateTracker) save(key string, state *HandState) error {
	stateInBytes, err := proto.Marshal(state)
	if err != nil {
		return err
	}
	err = r.rdclient.Set(context.Background(), key, stateInBytes, 0).Err()
	return err
}

func (r *RedisHandStateTracker) Remove(gameCode string, handID uint32) error {
	key := fmt.Sprintf("%s|%d", gameCode, handID)
	return r.remove(key)
}

func (r *RedisHandStateTracker) RemoveClone(gameCode string, handID uint32) error {
	key := fmt.Sprintf("%s|%d|clone", gameCode, handID)
	return r.remove(key)
}

func (r *RedisHandStateTracker) remove(key string) error {
	err := r.rdclient.Del(context.Background(), key).Err()
	return err
}
