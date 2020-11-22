package rest

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
	"voyager.com/server/nats"
)

var restLogger = log.With().Str("logger_name", "game::rest").Logger()
var natsGameManager *nats.GameManager

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

	r.GET("/someJSON", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "hey", "status": http.StatusOK})
	})
	r.POST("/new-game", newGame)
	r.POST("/player-update", playerUpdate)
	r.POST("/game-update-status", gameUpdateStatus)
	r.Run(":8080")
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

	// initialize nats game
	_, e := natsGameManager.NewGame(gameConfig.ClubId, gameConfig.GameId, &gameConfig)
	if e != nil {
		msg := fmt.Sprintf("Unable to initialize nats game: %v", e)
		restLogger.Error().Msg(msg)
		panic(msg)
	}

	c.JSON(http.StatusOK, tableStatus{
		GameID:      gameConfig.GameId,
		TableStatus: uint32(game.TableStatus_TABLE_STATUS_WAITING_TO_BE_STARTED),
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
	natsGameManager.GameStatusChanged(gameStatus.GameId, game.GameStatus(gameStatus.GameStatus))
}
