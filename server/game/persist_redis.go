package game

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
	"github.com/golang/protobuf/proto"
)

type RedisHandStateTracker struct {
	rdclient *redis.Client
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

func (r *RedisHandStateTracker) Load(gameCode string) (*HandState, error) {
	return r.load(gameCode)
}

func (r *RedisHandStateTracker) load(key string) (*HandState, error) {
	handStateBytes, err := r.rdclient.Get(context.Background(), key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("Hand state for Key: %s is not found", key)
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

func (r *RedisHandStateTracker) Save(gameCode string, state *HandState) error {
	return r.save(gameCode, state)
}

func (r *RedisHandStateTracker) save(key string, state *HandState) error {
	stateInBytes, err := proto.Marshal(state)
	if err != nil {
		return err
	}
	err = r.rdclient.Set(context.Background(), key, stateInBytes, 0).Err()
	return err
}

func (r *RedisHandStateTracker) Remove(gameCode string) error {
	return r.remove(gameCode)
}

func (r *RedisHandStateTracker) remove(key string) error {
	err := r.rdclient.Del(context.Background(), key).Err()
	return err
}
