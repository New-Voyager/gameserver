package rest

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"voyager.com/logging"
	"voyager.com/timer/internal/timer"
)

var (
	restLogger        = logging.GetZeroLogger("rest::rest", nil)
	timeoutController = timer.GetController()
)

func RunServer(portNo uint) {
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/ready", checkReady)
	r.POST("/start-timer", startTimer)
	r.POST("/cancel-timer", cancelTimer)
	r.Run(fmt.Sprintf(":%d", portNo))
}

func checkReady(c *gin.Context) {
	type resp struct {
		Status string `json:"status"`
	}
	c.JSON(http.StatusOK, resp{Status: "OK"})
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

	restLogger.Info().
		Uint64(logging.GameIDKey, gameID).
		Uint64(logging.PlayerIDKey, playerID).
		Str(logging.TimerPurposeKey, purpose).
		Msgf("start-timer timeout: %s (seconds)", timeoutAtStr)

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

	restLogger.Info().
		Uint64(logging.GameIDKey, gameID).
		Uint64(logging.PlayerIDKey, playerID).
		Str(logging.TimerPurposeKey, purpose).
		Msgf("cancel-timer")

	timeoutController.CancelTimer(gameID, playerID, purpose)
}
