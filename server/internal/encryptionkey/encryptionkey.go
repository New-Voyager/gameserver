package encryptionkey

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"voyager.com/logging"
)

var encryptionKeyCacheLogger = logging.GetZeroLogger("internal::encryptionkey", nil)

type Cache struct {
	cache            *lru.Cache
	apiServerURL     string
	maxFetchRetries  int
	retryDelayMillis int
}

func NewCache(size int, apiServerURL string) (*Cache, error) {
	c, err := lru.New(size)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to initialize cache")
	}
	return &Cache{
		cache:            c,
		apiServerURL:     apiServerURL,
		maxFetchRetries:  2,
		retryDelayMillis: 1000,
	}, nil
}

func (c *Cache) Get(playerID uint64) (string, error) {
	v, exists := c.cache.Get(playerID)
	if !exists {
		key, err := c.fetch(playerID)
		if err != nil {
			return "", errors.Wrap(err, "Unable to fetch encryption key")
		}
		c.Add(playerID, key)
		return key, nil
	}
	return v.(string), nil
}

func (c *Cache) Add(playerID uint64, encryptionKey string) {
	c.cache.Add(playerID, encryptionKey)
}

func (c *Cache) fetch(playerID uint64) (string, error) {
	url := fmt.Sprintf("%s/internal/get-encryption-key/playerId/%d", c.apiServerURL, playerID)

	retries := 0
	resp, err := http.Get(url)
	for err != nil && retries < int(c.maxFetchRetries) {
		retries++
		encryptionKeyCacheLogger.Error().
			Msgf("Error in get %s: %s. Retrying (%d/%d)", url, err, retries, c.maxFetchRetries)
		time.Sleep(time.Duration(c.retryDelayMillis) * time.Millisecond)
		resp, err = http.Get(url)
	}

	if err != nil {
		return "", errors.Wrapf(err, "Error from http get %s", url)
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Error while reading response body from %s", url)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Received http status %d from %s. Response body: %s", resp.StatusCode, url, string(bodyBytes))
	}

	type payload struct {
		Status string
		Key    string
	}
	var body payload
	err = json.Unmarshal(bodyBytes, &body)
	if err != nil {
		return "", errors.Wrapf(err, "Could not unmarshal response body from %s", url)
	}

	return body.Key, nil
}
