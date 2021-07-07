package game

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
	"github.com/golang/protobuf/proto"
)

type RedisHandsSetupTracker struct {
	rdclient *redis.Client
}

func NewRedisHandsSetupTracker(redisURL string, redisPW string, redisDB int) *RedisHandsSetupTracker {
	rdclient := redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: redisPW,
		DB:       redisDB,
	})
	return &RedisHandsSetupTracker{
		rdclient: rdclient,
	}
}

func (s *RedisHandsSetupTracker) Load(gameCode string) (*TestHandSetup, error) {
	return s.load(s.getKey(gameCode))
}

func (s *RedisHandsSetupTracker) load(key string) (*TestHandSetup, error) {
	handStateBytes, err := s.rdclient.Get(context.Background(), key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("Hands setup for Key: %s is not found", key)
	} else if err != nil {
		return nil, err
	}
	handsSetup := &TestHandSetup{}
	err = proto.Unmarshal([]byte(handStateBytes), handsSetup)
	if err != nil {
		return nil, err
	}
	return handsSetup, nil
}

func (s *RedisHandsSetupTracker) Save(gameCode string, handsSetup *TestHandSetup) error {
	return s.save(s.getKey(gameCode), handsSetup)
}

func (s *RedisHandsSetupTracker) save(key string, handsSetup *TestHandSetup) error {
	stateInBytes, err := proto.Marshal(handsSetup)
	if err != nil {
		return err
	}
	err = s.rdclient.Set(context.Background(), key, stateInBytes, 0).Err()
	return err
}

func (s *RedisHandsSetupTracker) Remove(gameCode string) error {
	return s.remove(s.getKey(gameCode))
}

func (s *RedisHandsSetupTracker) remove(key string) error {
	err := s.rdclient.Del(context.Background(), key).Err()
	return err
}

func (s *RedisHandsSetupTracker) getKey(gameCode string) string {
	return fmt.Sprintf("%s:NEXT_DECK", gameCode)
}
