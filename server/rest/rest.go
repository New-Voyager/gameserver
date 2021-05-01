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
	"voyager.com/server/timer"
)

var restLogger = log.With().Str("logger_name", "game::rest").Logger()
var natsGameManager *nats.GameManager
var timeoutController = timer.GetController()

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
func RunRestServer(gameManager *nats.GameManager) {
	natsGameManager = gameManager
	r := gin.Default()
	//r.Use(JSONAppErrorReporter())

	r.POST("/new-game", newGame)
	r.POST("/player-update", playerUpdate)
	r.POST("/game-update-status", gameUpdateStatus)
	r.POST("/pending-updates", gamePendingUpdates)
	r.GET("/current-hand-log", gameCurrentHandLog)
	r.POST("/table-update", tableUpdate)

	r.POST("/start-timer", startTimer)
	r.POST("/cancel-timer", cancelTimer)

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
	restLogger.Info().Msgf("New game is received")
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

	fmt.Printf("new-game payload: %+v\n", gameConfig)

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

func playerUpdate(c *gin.Context) {
	var playerUpdate nats.PlayerUpdate
	var err error

	err = c.BindJSON(&playerUpdate)
	if err != nil {
		restLogger.Error().Msg(fmt.Sprintf("Failed to read player update message. Error: %s", err.Error()))
		return
	}

	log.Info().Uint64("gameId", playerUpdate.GameId).Msg(fmt.Sprintf("Player: %d seatNo: %d is updated: %v", playerUpdate.PlayerId, playerUpdate.SeatNo, playerUpdate))
	natsGameManager.PlayerUpdate(playerUpdate.GameId, &playerUpdate)
}

func gameUpdateStatus(c *gin.Context) {
	var gameStatus nats.GameStatus
	var err error

	err = c.BindJSON(&gameStatus)
	if err != nil {
		restLogger.Error().Msg(fmt.Sprintf("Failed to read game update message. Error: %s", err.Error()))
		return
	}
	log.Info().Uint64("gameId", gameStatus.GameId).Msg(fmt.Sprintf("New game status: %d", gameStatus.GameStatus))
	natsGameManager.GameStatusChanged(gameStatus.GameId, gameStatus)
}

func gamePendingUpdates(c *gin.Context) {
	gameIDStr := c.Query("game-id")
	if gameIDStr == "" {
		c.String(400, "Failed to read game-id param from pending-updates endpoint")
	}

	started := c.Query("started")
	done := c.Query("done")

	gameID, err := strconv.ParseUint(gameIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse game-id [%s] from pending-updates endpoint.", gameIDStr)
	}
	if started != "" {
		// API server started processing pending updates
		//natsGameManager.GamePendingUpdatesStarted(gameID)
		panic("Not implemented")
	} else if done != "" {
		restLogger.Info().Msgf("****** Pending updates done for game %d", gameID)
		// pending updates done, game can resume
		natsGameManager.PendingUpdatesDone(gameID)
	}
}

func startTimer(c *gin.Context) {
	gameIDStr := c.Query("game-id")
	if gameIDStr == "" {
		c.String(400, "Failed to read game-id param from start-timer endpoint")
	}
	playerIDStr := c.Query("player-id")
	if playerIDStr == "" {
		c.String(400, "Failed to read player-id param from start-timer endpoint.")
	}
	purpose := c.Query("purpose")
	if purpose == "" {
		c.String(400, "Failed to read purpose param from start-timer endpoint.")
	}
	timeoutAtStr := c.Query("timeout-at")
	if timeoutAtStr == "" {
		c.String(400, "Failed to read timeout-at param from start-timer endpoint.")
	}
	gameID, err := strconv.ParseUint(gameIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse game-id [%s] from start-time endpoint.", gameIDStr)
	}
	playerID, err := strconv.ParseUint(playerIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse player-id [%s] from start-time endpoint.", playerIDStr)
	}
	timeoutAt, err := strconv.ParseInt(timeoutAtStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse timeout-at [%s] from start-time endpoint.", timeoutAtStr)
	}

	restLogger.Info().Msgf("start-timer game id: %s player id: %s purpose: %s timeout: %s (seconds)", gameIDStr, playerIDStr, purpose, timeoutAtStr)

	timeoutController.AddTimer(gameID, playerID, purpose, timeoutAt)
}

func cancelTimer(c *gin.Context) {
	gameIDStr := c.Query("game-id")
	if gameIDStr == "" {
		c.String(400, "Failed to read game-id param from cancel-timer endpoint")
	}
	playerIDStr := c.Query("player-id")
	if playerIDStr == "" {
		c.String(400, "Failed to read player-id param from cancel-timer endpoint.")
	}
	purpose := c.Query("purpose")
	if purpose == "" {
		c.String(400, "Failed to read purpose param from cancel-timer endpoint.")
	}
	gameID, err := strconv.ParseUint(gameIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse game-id [%s] from start-time endpoint.", gameIDStr)
	}
	playerID, err := strconv.ParseUint(playerIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse player-id [%s] from start-time endpoint.", playerIDStr)
	}

	restLogger.Info().Msgf("cancel-timer game id: %s player id: %s purpose: %s", gameID, playerID, purpose)

	timeoutController.CancelTimer(gameID, playerID, purpose)
}

func gameCurrentHandLog(c *gin.Context) {
	gameIDStr := c.Query("game-id")
	gameCode := c.Query("game-code")
	if gameIDStr == "" {
		if gameCode == "" {
			c.String(400, "Either game code or game id should be specified (e.g /current-hand-log?game-code=<>")
			return
		}
	}

	gameID, err := strconv.ParseUint(gameIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse game-id [%s] from pending-updates endpoint.", gameIDStr)
	}
	log := natsGameManager.GetCurrentHandLog(gameID, gameCode)
	c.JSON(http.StatusOK, log)
}

func tableUpdate(c *gin.Context) {
	var tableUpdate nats.TableUpdate
	var err error

	err = c.BindJSON(&tableUpdate)
	if err != nil {
		restLogger.Error().Msg(fmt.Sprintf("Failed to read table update message. Error: %s", err.Error()))
		return
	}

	log.Info().Uint64("gameId", tableUpdate.GameId).Msg(fmt.Sprintf("Type: %s", tableUpdate.Type))
	natsGameManager.TableUpdate(tableUpdate.GameId, &tableUpdate)
}
