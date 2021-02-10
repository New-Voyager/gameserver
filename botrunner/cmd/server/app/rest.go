package app

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"voyager.com/botrunner/internal/game"
	"voyager.com/botrunner/internal/util"
)

var (
	restLogger      = log.With().Str("logger_name", "app::rest").Logger()
	baseLogDir      = "log"
	humanGameScript = "botrunner_scripts/human-game.yaml"
)

// RunRestServer registers http endpoints and handlers and runs the server.
func RunRestServer(portNo uint, logDir string) {
	if logDir != "" {
		baseLogDir = logDir
	}

	r := gin.Default()

	r.POST("/apply", apply)
	r.POST("/delete", deleteBatch)
	r.POST("/delete-all", deleteAll)
	r.POST("/join-human-game", joinHumanGame)
	r.POST("/delete-human-game", deleteHumanGame)
	r.Run(fmt.Sprintf(":%d", portNo))
}

// BatchConf is the payload for the '/apply' and '/delete' endpoints.
// '/delete' only takes BatchID and ignores the other fields.
type BatchConf struct {
	// BatchID is the unique name given to a group of BotRunners.
	BatchID string `json:"batchId"`

	// Script is the BotRunner script YAML file used by the BotRunners in this batch.
	Script string `json:"script"`

	// NumGames is the number of desired BotRunners to run in this batch.
	NumGames *uint32 `json:"numGames"`

	// Number of seconds (in float) to pause between launching BotRunners.
	LaunchInterval *float32 `json:"launchInterval"`
}

func apply(c *gin.Context) {
	var payload BatchConf
	err := c.BindJSON(&payload)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to parse payload. Error: %s", err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	err = validateApplyPayload(payload)
	if err != nil {
		restLogger.Error().Msg(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if payload.BatchID == "" {
		payload.BatchID = "default_group"
	}

	launcher := GetLauncher()

	var botrunnerConf *game.BotRunnerConfig
	if !launcher.BatchExists(payload.BatchID) && payload.Script == "" {
		errMsg := "A botrunner script must be provided to start a new batch."
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	if !launcher.BatchExists(payload.BatchID) {
		botrunnerConf, err = game.ParseYAMLConfig(payload.Script)
		if err != nil {
			errMsg := fmt.Sprintf("Error while parsing config file. Error: %s", err)
			restLogger.Error().Msg(errMsg)
			c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
			return
		}
	}

	err = launcher.ApplyToBatch(payload.BatchID, botrunnerConf, *payload.NumGames, payload.LaunchInterval)
	if err != nil {
		errMsg := fmt.Sprintf("Error while applying config. Error: %s", err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Accepted"})
}

func deleteBatch(c *gin.Context) {
	var batchConf BatchConf
	err := c.BindJSON(&batchConf)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to parse payload. Error: %s", err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}
	restLogger.Info().Msgf("/delete batch ID: [%s]\n", batchConf.BatchID)
	launcher := GetLauncher()
	err = launcher.StopBatch(batchConf.BatchID)
	if err != nil {
		errMsg := fmt.Sprintf("Error while deleting batch [%s]. Error: %s", batchConf.BatchID, err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "Accepted"})
}

func deleteAll(c *gin.Context) {
	launcher := GetLauncher()
	err := launcher.StopAll()
	if err != nil {
		errMsg := fmt.Sprintf("Error while deleting batches. Error: %s", err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "Accepted"})
}

func validateApplyPayload(payload BatchConf) error {
	errors := make([]string, 0)
	if payload.BatchID != "" && !util.IsAlphaNumericUnderscore(payload.BatchID) {
		errors = append(errors, "batchId should only contain alphanumeric chars and underscore")
	}
	if payload.NumGames == nil {
		errors = append(errors, "numGames is missing")
	}
	if payload.LaunchInterval != nil && *payload.LaunchInterval < 0 {
		errors = append(errors, "launchInterval must be >= 0")
	}
	if len(errors) > 0 {
		return fmt.Errorf(strings.Join(errors, "\n"))
	}
	return nil
}

func joinHumanGame(c *gin.Context) {
	clubCode := c.Query("club-code")
	if clubCode == "" {
		c.String(400, "Failed to read club-code param from join-hame endpoint")
	}
	gameCode := c.Query("game-code")
	if gameCode == "" {
		c.String(400, "Failed to read game-code param from join-hame endpoint.")
	}
	botrunnerConf, err := game.ParseYAMLConfig(humanGameScript)
	if err != nil {
		errMsg := fmt.Sprintf("Error while parsing config file %s. Error: %s", humanGameScript, err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	launcher := GetLauncher()
	err = launcher.JoinHumanGame(clubCode, gameCode, botrunnerConf)
	if err != nil {
		errMsg := fmt.Sprintf("Error while joining human game. Error: %s", err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Accepted"})
}

func deleteHumanGame(c *gin.Context) {
	gameCode := c.Query("game-code")
	if gameCode == "" {
		c.String(400, "Failed to read game-code param from join-hame endpoint.")
	}

	launcher := GetLauncher()
	err := launcher.DeleteHumanGame(gameCode)
	if err != nil {
		errMsg := fmt.Sprintf("Error while deleting human game tracker. Error: %s", err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Accepted"})
}
