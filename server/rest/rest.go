package rest

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"voyager.com/server/crashtest"
	"voyager.com/server/game"
	"voyager.com/server/nats"
	"voyager.com/server/util"
)

var restLogger = log.With().Str("logger_name", "game::rest").Logger()
var natsGameManager *nats.GameManager
var onEndSystemTest func()

//
// APP error definition
//
type appError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type tableStatus struct {
	GameID      uint64 `json:"gameId"`
	TableStatus uint32 `json:"tableStatus"`
}

/*
//
// Middleware Error Handler in server package
//
func JSONAppErrorReporter() gin.HandlerFunc {
	return jsonAppErrorReporterT(gin.ErrorTypeAny)
}

func jsonAppErrorReporterT(errType gin.ErrorType) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		detectedErrors := c.Errors.ByType(errType)

		restLogger.Debug().Msg("Handling error")
		if len(detectedErrors) > 0 {
			err := detectedErrors[0].Err
			var parsedError *appError
			switch err.(type) {
			case *appError:
				parsedError = err.(*appError)
			default:
				parsedError = &appError{
					Code:    http.StatusInternalServerError,
					Message: "Internal Server Error",
				}
			}
			// Put the error into response
			c.IndentedJSON(parsedError.Code, parsedError)
			c.Abort()
			return
		}

	}
}
*/
func RunRestServer(gameManager *nats.GameManager, endSystemTestCallback func()) {
	natsGameManager = gameManager
	r := gin.Default()
	//r.Use(JSONAppErrorReporter())

	r.POST("/new-game", newGame)
	r.POST("/resume-game", resumeGame)
	r.GET("/current-hand-log", gameCurrentHandLog)
	if util.Env.IsSystemTest() {
		onEndSystemTest = endSystemTestCallback
		r.POST("/end-system-test", endSystemTest)
	}

	// Intentionally crash the process for testing
	r.POST("/setup-crash", setupCrash)
	r.Run(":8080")
}

func setupCrash(c *gin.Context) {
	type Payload struct {
		GameCode   string `json:"gameCode"`
		CrashPoint string `json:"crashPoint"`
		PlayerID   uint64 `json:"playerId"`
	}
	var payload Payload
	err := c.BindJSON(&payload)
	if err != nil {
		restLogger.Error().Msgf("Unable to parse crash configuration. Error: %v", err)
		c.IndentedJSON(http.StatusInternalServerError, appError{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		})
		c.Error(err)
		return
	}

	restLogger.Info().Msgf("Received request to crash the server at [%s] player [%d]", payload.CrashPoint, payload.PlayerID)
	err = crashtest.Set(payload.GameCode, crashtest.CrashPoint(payload.CrashPoint), payload.PlayerID)
	if err != nil {
		restLogger.Error().Msgf("Unable to setup server for crash. Error: %v", err)
		c.IndentedJSON(http.StatusInternalServerError, appError{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		})
		c.Error(err)
		return
	}
}

func newGame(c *gin.Context) {
	restLogger.Debug().Msgf("New game is received")
	var gameConfig game.GameConfig
	var err error
	err = c.BindJSON(&gameConfig)
	if err != nil {
		restLogger.Error().Msgf("Failed to parse game configuration. Error: %v", err)
		c.IndentedJSON(http.StatusInternalServerError, appError{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		})
		c.Error(err)
		return
	}

	restLogger.Debug().Msgf("new-game payload: %+v", gameConfig)

	// initialize nats game
	_, e := natsGameManager.NewGame(gameConfig.ClubId, gameConfig.GameId, &gameConfig)
	if e != nil {
		msg := fmt.Sprintf("Unable to initialize nats game: %v", e)
		restLogger.Error().Msg(msg)
		panic(msg)
	}

	c.JSON(http.StatusOK, tableStatus{
		GameID:      gameConfig.GameId,
		TableStatus: uint32(game.TableStatus_WAITING_TO_BE_STARTED),
	})
}

func resumeGame(c *gin.Context) {
	gameIDStr := c.Query("game-id")
	if gameIDStr == "" {
		c.String(400, "Failed to read game-id param from pending-updates endpoint")
	}
	gameID, err := strconv.ParseUint(gameIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse game-id [%s] from pending-updates endpoint.", gameIDStr)
	}

	restLogger.Debug().Msgf("****** Pending updates done for game %s", gameIDStr)
	// pending updates done, game can resume
	natsGameManager.ResumeGame(gameID)
}

func gameCurrentHandLog(c *gin.Context) {
	gameIDStr := c.Query("game-id")
	if gameIDStr == "" {
		c.String(400, "Game id should be specified (e.g /current-hand-log?game-id=<>")
		return
	}

	gameID, err := strconv.ParseUint(gameIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse game-id [%s] from current hand log endpoint.", gameIDStr)
	}
	log := natsGameManager.GetCurrentHandLog(gameID)
	c.JSON(http.StatusOK, log)
}

func endSystemTest(c *gin.Context) {
	onEndSystemTest()
	return
}
