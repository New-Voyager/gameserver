package app

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"voyager.com/botrunner/internal/caches"
	"voyager.com/botrunner/internal/util"
	"voyager.com/gamescript"
	"voyager.com/logging"
)

var (
	restLogger      = logging.GetZeroLogger("app::rest", nil)
	baseLogDir      = "log"
	humanGameScript = "botrunner_scripts/human_game/human-game.yaml"
	playersConfig   = "botrunner_scripts/players/default.yaml"
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
	r.POST("/start-app-game", startAppGame)
	r.GET("/app-games", listAppGames)
	r.Run(fmt.Sprintf(":%d", portNo))
}

// BatchConf is the payload for the '/apply' and '/delete' endpoints.
// '/delete' only takes BatchID and ignores the other fields.
type BatchConf struct {
	// BatchID is the unique name given to a group of BotRunners.
	BatchID string `json:"batchId"`

	// Script is the game script YAML file used by the BotRunners in this batch.
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

	var script *gamescript.Script
	var players *gamescript.Players
	if !launcher.BatchExists(payload.BatchID) && payload.Script == "" {
		errMsg := "A botrunner script must be provided to start a new batch."
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	if !launcher.BatchExists(payload.BatchID) {
		script, err = gamescript.ReadGameScript(payload.Script)
		if err != nil {
			errMsg := fmt.Sprintf("Error while parsing script file. Error: %s", err)
			restLogger.Error().Msg(errMsg)
			c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
			return
		}
		players, err = gamescript.ReadPlayersConfig(playersConfig)
		if err != nil {
			errMsg := fmt.Sprintf("Error while parsing players file. Error: %s", err)
			restLogger.Error().Msg(errMsg)
			c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
			return
		}
	}

	err = launcher.ApplyToBatch(payload.BatchID, players, script, *payload.NumGames, payload.LaunchInterval)
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

func listAppGames(c *gin.Context) {
	type listItem struct {
		AppGame    string
		ScriptFile string
	}
	var scripts []listItem
	m, err := listAppGameScripts()
	if err != nil {
		restLogger.Error().Msg(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for appGameTitle, scriptFile := range m {
		scripts = append(scripts, listItem{ScriptFile: scriptFile, AppGame: appGameTitle})
	}
	c.JSON(http.StatusOK, gin.H{"scripts": scripts})
}

// app game name -> script file
func listAppGameScripts() (map[string]string, error) {
	fileNames, err := util.GetFilesInDir("botrunner_scripts")
	if err != nil {
		return nil, fmt.Errorf("Error while listing files in botrunner_scripts directory. Error: %s", err)
	}
	res := make(map[string]string)
	for _, scriptFile := range fileNames {
		if strings.HasSuffix(scriptFile, "/players/default.yaml") {
			continue
		}
		appGameTitle, err := getAppGameTitle(scriptFile)
		if err != nil {
			return nil, fmt.Errorf("Error while parsing game title for script %s. Error: %s", scriptFile, err)
		}
		if appGameTitle == "" {
			continue
		}
		res[appGameTitle] = scriptFile
	}
	return res, nil
}

func getGameTitle(scriptFile string) (string, error) {
	script, err := gamescript.ReadGameScript(scriptFile)
	if err != nil {
		return "", errors.Wrap(err, "Error while parsing script file")
	}
	return script.Game.Title, nil
}

func getAppGameTitle(scriptFile string) (string, error) {
	script, err := gamescript.ReadGameScript(scriptFile)
	if err != nil {
		return "", errors.Wrap(err, "Error while parsing script file")
	}
	return script.AppGame, nil
}

func joinHumanGame(c *gin.Context) {
	clubCode := c.Query("club-code")
	// if clubCode == "" {
	// 	//c.String(400, "Failed to read club-code param from join-human-game endpoint")
	// }
	gameCode := c.Query("game-code")
	if gameCode == "" {
		c.String(400, "Failed to read game-code param from join-human-game endpoint.")
	}
	if clubCode == "null" {
		clubCode = ""
	}
	gameIDStr := c.Query("game-id")
	if gameIDStr == "" {
		c.String(400, "Failed to read game-id param from join-human-game endpoint")
	}
	gameID, err := strconv.ParseUint(gameIDStr, 10, 64)
	if err != nil {
		c.String(400, "Failed to parse game-id [%s] from join-human-game endpoint.", gameIDStr)
	}

	demoGame := false
	demoGameStr := c.Query("demo-game")
	if demoGameStr != "" && demoGameStr == "true" {
		demoGame = true
	}

	err = caches.GameCodeCache.Add(gameID, gameCode)
	if err != nil {
		restLogger.Error().Msgf(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	players, err := gamescript.ReadPlayersConfig(playersConfig)
	if err != nil {
		errMsg := fmt.Sprintf("Error while parsing players config file %s. Error: %s", playersConfig, err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}
	script, err := gamescript.ReadGameScript(humanGameScript)
	if err != nil {
		errMsg := fmt.Sprintf("Error while parsing script file %s. Error: %s", humanGameScript, err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	launcher := GetLauncher()
	err = launcher.JoinHumanGame(clubCode, gameID, gameCode, players, script, demoGame)
	if err != nil {
		errMsg := fmt.Sprintf("Error while joining human game. Error: %s", err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Accepted"})
}

func startAppGame(c *gin.Context) {
	type Payload struct {
		ClubCode string `json:"clubCode"`
		Name     string `json:"name"`
	}
	var payload Payload
	err := c.BindJSON(&payload)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to parse payload. Error: %s", err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}
	name := payload.Name
	if name == "" {
		c.String(400, "Failed to read name param from start-app-game endpoint.")
		return
	}
	clubCode := payload.ClubCode
	if clubCode == "" {
		c.String(400, "Failed to read club-code param from start-app-game endpoint")
		return
	}
	m, err := listAppGameScripts()
	if err != nil {
		restLogger.Error().Msg(err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	scriptFile, ok := m[name]
	if !ok {
		errMsg := fmt.Sprintf("Unable to find script file for app game %s", name)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}
	script, err := gamescript.ReadGameScript(scriptFile)
	if err != nil {
		errMsg := fmt.Sprintf("Error while parsing script file %s. Error: %s", scriptFile, err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}
	players, err := gamescript.ReadPlayersConfig(playersConfig)
	if err != nil {
		errMsg := fmt.Sprintf("Error while parsing players config file %s. Error: %s", playersConfig, err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}
	for i, _ := range script.Hands {
		script.Hands[i].Setup.ResultPauseTime = 3000
	}
	launcher := GetLauncher()
	err = launcher.StartAppGame(clubCode, name, players, script)
	if err != nil {
		errMsg := fmt.Sprintf("Error while starting app game. Error: %s", err)
		restLogger.Error().Msg(errMsg)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Accepted"})
}

func deleteHumanGame(c *gin.Context) {
	gameCode := c.Query("game-code")
	if gameCode == "" {
		c.String(400, "Failed to read game-code param from delete-human-game endpoint.")
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
