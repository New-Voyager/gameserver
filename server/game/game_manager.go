package game

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"voyager.com/server/internal"
	"voyager.com/server/util"
)

var GameManager *Manager

func CreateGameManager(isScriptTest bool, delays Delays) *Manager {
	if GameManager != nil {
		return GameManager
	}

	var apiServerURL string
	var db *sqlx.DB
	var err error
	if !isScriptTest {
		apiServerURL = util.GameServerEnvironment.GetApiServerUrl()

		db, err = sqlx.Open("postgres", internal.GetConnStr())
		if err != nil {
			panic(errors.Wrap(err, "Unable to create sqlx handle to postgres"))
		}
		err = db.Ping()
		if err != nil {
			panic(errors.Wrap(err, "Unable to verify postgres connection"))
		}
	}

	var redisHost = util.GameServerEnvironment.GetRedisHost()
	var redisPort = util.GameServerEnvironment.GetRedisPort()
	var redisPW = util.GameServerEnvironment.GetRedisPW()
	var redisDB = util.GameServerEnvironment.GetRedisDB()
	handSetupPersist := NewRedisHandsSetupTracker(fmt.Sprintf("%s:%d", redisHost, redisPort), redisPW, redisDB)

	var handPersist PersistHandState
	var persistMethod = util.GameServerEnvironment.GetPersistMethod()
	if persistMethod == "redis" {
		handPersist = NewRedisHandStateTracker(fmt.Sprintf("%s:%d", redisHost, redisPort), redisPW, redisDB)
	} else {
		handPersist = NewMemoryHandStateTracker()
	}

	gm, err := NewGameManager(isScriptTest, apiServerURL, handPersist, handSetupPersist, db, delays)
	if err != nil {
		panic(err)
	}

	GameManager = gm
	return GameManager
}
