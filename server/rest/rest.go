package rest

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"voyager.com/logging"
	"voyager.com/server/crashtest"
	"voyager.com/server/game"
	"voyager.com/server/internal/caches"
	"voyager.com/server/nats"
	"voyager.com/server/util"
)

var restLogger = logging.GetZeroLogger("game::rest", nil)
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
	r := gin.New()
	r.Use(gin.Recovery())
	//r.Use(JSONAppErrorReporter())

	r.GET("/ready", checkReady)
	r.POST("/new-game", newGame)
	r.POST("/resume-game", resumeGame)
	r.POST("/left-game", leftGame)
	r.POST("/end-game", endGame)
	r.GET("/games", getGames)
	r.GET("/current-hand-log", gameCurrentHandLog)
	if util.Env.IsSystemTest() {
		onEndSystemTest = endSystemTestCallback
		r.POST("/end-system-test", endSystemTest)
	}

	// Intentionally crash the process for testing
	r.POST("/setup-crash", setupCrash)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.Run(":8080")
}

func checkReady(c *gin.Context) {
	type resp struct {
		Status string `json:"status"`
	}
	c.JSON(http.StatusOK, resp{Status: "OK"})
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
	type payload struct {
		GameID    uint64 `json:"gameId"`
		GameCode  string `json:"gameCode"`
		IsRestart bool   `json:"isRestart"`
	}
	var gameConfig payload
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

	gameID := gameConfig.GameID
	gameCode := gameConfig.GameCode

	restLogger.Info().
		Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Msgf("New game is received: %+v", gameConfig)

	caches.GameCodeCache.Add(gameID, gameCode)
	util.Metrics.NewGameReceived()

	// initialize nats game
	_, err = natsGameManager.NewGame(gameID, gameCode)
	if err != nil {
		msg := fmt.Sprintf("Unable to initialize nats game: %v", err)
		restLogger.Error().Msg(msg)
		panic(msg)
	}

	if gameConfig.IsRestart {
		// This game is being restarted due to game server crash, etc.
		// Need to resume from where it left off.
		restLogger.Debug().
			Uint64(logging.GameIDKey, gameID).
			Str(logging.GameCodeKey, gameCode).
			Msgf("Resuming game due to restart")
		natsGameManager.ResumeGame(gameID)
	}

	// TODO: Returning table status probably doesn't make sense.
	c.JSON(http.StatusOK, tableStatus{
		GameID:      gameID,
		TableStatus: uint32(game.TableStatus_WAITING_TO_BE_STARTED),
	})
}

func resumeGame(c *gin.Context) {
	gameIDStr := c.Query("game-id")
	if gameIDStr == "" {
		c.String(400, "Failed to read game-id param from resume-game endpoint")
	}
	gameID, err := strconv.ParseUint(gameIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse game-id [%s] from resume-game endpoint.", gameIDStr)
	}

	gameCode, ok := caches.GameCodeCache.GameIDToCode(gameID)
	if !ok {
		// Should not get here.
		gameCode = ""
		restLogger.Warn().Uint64(logging.GameIDKey, gameID).Msgf("No game code found in cache while resuming game")
	}
	restLogger.Debug().
		Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Msgf("Resuming game")
	natsGameManager.ResumeGame(gameID)
}

func endGame(c *gin.Context) {
	gameIDStr := c.Query("game-id")
	if gameIDStr == "" {
		c.String(400, "Failed to read game-id param from end-game endpoint")
	}
	gameID, err := strconv.ParseUint(gameIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse game-id [%s] from end-game endpoint.", gameIDStr)
	}

	gameCode, ok := caches.GameCodeCache.GameIDToCode(gameID)
	if !ok {
		// Should not get here.
		gameCode = ""
		restLogger.Warn().Uint64(logging.GameIDKey, gameID).Msgf("No game code found in cache while ending game")
	}
	restLogger.Debug().
		Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Msgf("Ending game")
	natsGameManager.EndNatsGame(gameID)
}

func getGames(c *gin.Context) {
	type payload struct {
		Games []nats.GameListItem `json:"games"`
		Count int                 `json:"count"`
	}

	games, err := natsGameManager.GetGames()
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, appError{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		})
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, payload{
		Games: games,
		Count: len(games),
	})
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
	log, success := natsGameManager.GetCurrentHandLog(gameID)
	status := http.StatusOK
	if !success {
		status = http.StatusInternalServerError
	}
	c.JSON(status, log)
}

func endSystemTest(c *gin.Context) {
	onEndSystemTest()
	return
}

func leftGame(c *gin.Context) {
	gameIDStr := c.Query("game-id")
	if gameIDStr == "" {
		c.String(400, "Failed to read game-id param from resume-game endpoint")
	}
	gameID, err := strconv.ParseUint(gameIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse game-id [%s] from resume-game endpoint.", gameIDStr)
	}
	playerIDStr := c.Query("player-id")
	if gameIDStr == "" {
		c.String(400, "Failed to read player-id param from resume-game endpoint")
	}
	playerID, err := strconv.ParseUint(playerIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse game-id [%s] from resume-game endpoint.", playerIDStr)
	}

	gameCode, ok := caches.GameCodeCache.GameIDToCode(gameID)
	if !ok {
		// Should not get here.
		gameCode = ""
		restLogger.Warn().Uint64(logging.GameIDKey, gameID).Msgf("No game code found in cache while resuming game")
	}
	restLogger.Debug().
		Uint64(logging.GameIDKey, gameID).
		Str(logging.GameCodeKey, gameCode).
		Msgf("Player %d left the game %s", playerID, gameCode)
	natsGameManager.LeftGame(gameID, playerID)
}
