package game

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"voyager.com/server/internal"
	"voyager.com/server/util"
)

var GameManager *Manager

func CreateGameManager(isScriptTest bool, delays Delays) (*Manager, error) {
	if GameManager != nil {
		return GameManager, nil
	}

	var apiServerURL string
	var usersdb *sqlx.DB
	var crashdb *sqlx.DB
	var err error
	if !isScriptTest {
		apiServerURL = util.Env.GetApiServerUrl()

		crashdb, err = sqlx.Open("postgres", internal.GetGamesConnStr())
		if err != nil {
			return nil, errors.Wrap(err, "Unable to create sqlx handle to postgres")
		}
		err = crashdb.Ping()
		if err != nil {
			return nil, errors.Wrap(err, "Unable to verify postgres connection")
		}

		usersdb, err = sqlx.Open("postgres", internal.GetUsersConnStr())
		if err != nil {
			return nil, errors.Wrap(err, "Unable to create sqlx handle to postgres")
		}
		err = usersdb.Ping()
		if err != nil {
			return nil, errors.Wrap(err, "Unable to verify postgres connection")
		}
	}

	var redisHost = util.Env.GetRedisHost()
	var redisPort = util.Env.GetRedisPort()
	var redisPW = util.Env.GetRedisPW()
	var redisDB = util.Env.GetRedisDB()
	handSetupPersist := NewRedisHandsSetupTracker(fmt.Sprintf("%s:%d", redisHost, redisPort), redisPW, redisDB)

	var handPersist PersistHandState
	var persistMethod = util.Env.GetPersistMethod()
	if persistMethod == "redis" {
		handPersist = NewRedisHandStateTracker(fmt.Sprintf("%s:%d", redisHost, redisPort), redisPW, redisDB)
	} else {
		handPersist = NewMemoryHandStateTracker()
	}

	gm, err := NewGameManager(isScriptTest, apiServerURL, handPersist, handSetupPersist, usersdb, crashdb, delays)
	if err != nil {
		return nil, errors.Wrap(err, "Error in NewGameManager")
	}

	GameManager = gm
	return GameManager, nil
}
