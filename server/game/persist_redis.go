package game

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

type RedisHandStateTracker struct {
	rdclient     *redis.Client
	redisTimeout time.Duration
}

func NewRedisHandStateTracker(redisURL string, redisUser string, redisPW string, redisDB int, useSSL bool) (*RedisHandStateTracker, error) {
	var tlsConfig *tls.Config
	if useSSL {
		tlsConfig = &tls.Config{}
	}
	rdclient := redis.NewClient(&redis.Options{
		Addr:      redisURL,
		Username:  redisUser,
		Password:  redisPW,
		DB:        redisDB,
		TLSConfig: tlsConfig,
	})

	redisTimeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()
	_, err := rdclient.Ping(ctx).Result()
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to verify connection to Redis. Addr: %s", redisURL)
	}
	return &RedisHandStateTracker{
		rdclient:     rdclient,
		redisTimeout: redisTimeout,
	}, nil
}

func (r *RedisHandStateTracker) Load(gameCode string) (*HandState, error) {
	return r.load(gameCode)
}

func (r *RedisHandStateTracker) load(key string) (*HandState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.redisTimeout)
	defer cancel()
	handStateBytes, err := r.rdclient.Get(ctx, key).Result()
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
	ctx, cancel := context.WithTimeout(context.Background(), r.redisTimeout)
	defer cancel()
	err = r.rdclient.Set(ctx, key, stateInBytes, 0).Err()
	return err
}

func (r *RedisHandStateTracker) Remove(gameCode string) error {
	return r.remove(gameCode)
}

func (r *RedisHandStateTracker) remove(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.redisTimeout)
	defer cancel()
	err := r.rdclient.Del(ctx, key).Err()
	return err
}
