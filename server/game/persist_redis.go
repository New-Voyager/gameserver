package game

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"voyager.com/logging"
)

var redisLogger = logging.GetZeroLogger("game::persist_redis", nil)

type RedisHandStateTracker struct {
	rdclient        *redis.Client
	accessTimeout   time.Duration
	retryDelay      time.Duration
	abortRetryAfter time.Duration
}

const RedisKeyNotFound = PersistRedisError("Record not found")

type PersistRedisError string

func (e PersistRedisError) Error() string { return string(e) }

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

	accessTimeout := 5 * time.Second
	retryDelay := 2 * time.Second
	abortRetryAfter := 60 * time.Second

	err := pingWithTimeout(rdclient, accessTimeout)

	abortAt := time.Now().Add(abortRetryAfter)
	for retries := 1; err != nil && time.Now().Before(abortAt); retries++ {
		redisLogger.Error().Err(err).Msgf("Could not connect to Redis at %s. Retrying...%d", redisURL, retries)
		time.Sleep(retryDelay)
		err = pingWithTimeout(rdclient, accessTimeout)
		if err == nil {
			redisLogger.Info().Msg("Successfully connected to Redis")
		}
	}
	if err != nil {
		return nil, errors.Wrapf(err, "Unable to connect to Redis at %s", redisURL)
	}

	return &RedisHandStateTracker{
		rdclient:        rdclient,
		accessTimeout:   accessTimeout,
		retryDelay:      retryDelay,
		abortRetryAfter: abortRetryAfter,
	}, nil
}

func pingWithTimeout(client *redis.Client, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := client.Ping(ctx).Result()
	return err
}

func (r *RedisHandStateTracker) Load(gameCode string) (*HandState, error) {
	handStateStr, err := r.loadWithTimeout(gameCode)
	if err == redis.Nil {
		return nil, RedisKeyNotFound
	}

	abortAt := time.Now().Add(r.abortRetryAfter)
	for retries := 1; err != nil && time.Now().Before(abortAt); retries++ {
		redisLogger.Error().
			Err(err).
			Str(logging.GameCodeKey, gameCode).
			Msgf("Could not load hand state from Redis. Retrying...%d", retries)
		time.Sleep(r.retryDelay)
		handStateStr, err = r.loadWithTimeout(gameCode)
		if err == redis.Nil {
			redisLogger.Info().
				Str(logging.GameCodeKey, gameCode).
				Msg("Got nil hand state from Redis")
			return nil, RedisKeyNotFound
		}
		if err == nil {
			redisLogger.Info().
				Str(logging.GameCodeKey, gameCode).
				Msg("Successfully loaded hand state from Redis")
		}
	}
	if err != nil {
		redisLogger.Error().
			Err(err).
			Str(logging.GameCodeKey, gameCode).
			Msgf("Retry exhausted loading hand state")
		return nil, err
	}

	handState := &HandState{}
	err = proto.Unmarshal([]byte(handStateStr), handState)
	if err != nil {
		return nil, errors.Wrap(err, "Could not proto-unmarshal hand state from Redis")
	}
	return handState, nil
}

func (r *RedisHandStateTracker) loadWithTimeout(key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.accessTimeout)
	defer cancel()
	handStateStr, err := r.rdclient.Get(ctx, key).Result()
	return handStateStr, err
}

func (r *RedisHandStateTracker) Save(gameCode string, state *HandState) error {
	stateInBytes, err := proto.Marshal(state)
	if err != nil {
		return errors.Wrap(err, "Could not proto-marshal hand state")
	}

	err = r.saveWithTimeout(gameCode, stateInBytes)

	abortAt := time.Now().Add(r.abortRetryAfter)
	for retries := 1; err != nil && time.Now().Before(abortAt); retries++ {
		redisLogger.Error().
			Err(err).
			Str(logging.GameCodeKey, gameCode).
			Msgf("Could not save hand state to Redis. Retrying...%d", retries)
		time.Sleep(r.retryDelay)
		err = r.saveWithTimeout(gameCode, stateInBytes)
		if err == nil {
			redisLogger.Info().
				Str(logging.GameCodeKey, gameCode).
				Msg("Successfully saved hand state to Redis")
		}
	}
	if err != nil {
		redisLogger.Error().
			Err(err).
			Str(logging.GameCodeKey, gameCode).
			Msgf("Retry exhausted saving hand state")
	}

	return err
}

func (r *RedisHandStateTracker) saveWithTimeout(key string, handStatebytes []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.accessTimeout)
	defer cancel()
	return r.rdclient.Set(ctx, key, handStatebytes, 0).Err()
}

func (r *RedisHandStateTracker) Remove(gameCode string) error {
	return r.remove(gameCode)
}

func (r *RedisHandStateTracker) remove(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.accessTimeout)
	defer cancel()
	err := r.rdclient.Del(ctx, key).Err()
	return err
}
