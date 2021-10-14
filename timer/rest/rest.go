package rest

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"voyager.com/timer/internal/timer"
)

var (
	restLogger        = log.With().Str("logger_name", "rest::rest").Logger()
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

	restLogger.Info().Msgf("cancel-timer game id: %s player id: %s purpose: %s", gameIDStr, playerIDStr, purpose)

	timeoutController.CancelTimer(gameID, playerID, purpose)
}
