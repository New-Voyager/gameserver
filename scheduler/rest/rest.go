package rest

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"voyager.com/scheduler/internal/scheduler"
)

var (
	restLogger          = log.With().Str("logger_name", "rest::rest").Logger()
	schedulerController = scheduler.GetController()
)

func RunServer(portNo uint) {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/ready", checkReady)
	r.POST("/schedule-game-post-process", scheduleGamePostProcess)
	r.Run(fmt.Sprintf(":%d", portNo))
}

func checkReady(c *gin.Context) {
	type resp struct {
		Status string `json:"status"`
	}
	c.JSON(http.StatusOK, resp{Status: "OK"})
}

func scheduleGamePostProcess(c *gin.Context) {
	gameIDStr := c.Query("game-id")
	if gameIDStr == "" {
		c.String(400, "Failed to read game-id param from schedule-game-post-process endpoint")
	}
	gameID, err := strconv.ParseUint(gameIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse game-id [%s] from schedule-game-post-process endpoint.", gameIDStr)
	}

	schedulerController.SchedulePostProcess(gameID)
}
