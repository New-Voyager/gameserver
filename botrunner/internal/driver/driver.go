package driver

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/machinebox/graphql"
	natsgo "github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"voyager.com/botrunner/internal/game"
	"voyager.com/botrunner/internal/gql"
	"voyager.com/botrunner/internal/msgcheck"
	"voyager.com/botrunner/internal/player"
	"voyager.com/botrunner/internal/util"
)

// BotRunner is the main driver object that sets up the bots for a game.
type BotRunner struct {
	logger           *zerolog.Logger
	playerLogger     *zerolog.Logger
	msgCollector     *msgcheck.MsgCollector
	msgDumpFile      string
	expectedMsgsFile string
	clubCode         string
	botIsClubOwner   bool
	config           game.BotRunnerConfig
	gameCode         string
	botIsGameHost    bool
	currentHandNum   uint32
	bots             []*player.BotPlayer
	observerBot      *player.BotPlayer
	botsByName       map[string]*player.BotPlayer
	botsBySeat       map[uint32]*player.BotPlayer
	apiServerURL     string
	natsConn         *natsgo.Conn
	shouldTerminate  bool
}

// NewBotRunner creates new instance of BotRunner.
func NewBotRunner(clubCode string, gameCode string, config game.BotRunnerConfig, driverLogger *zerolog.Logger, playerLogger *zerolog.Logger, expectedMsgsFile string, msgDumpFile string) (*BotRunner, error) {
	natsURL := util.Env.GetNatsURL()
	nc, err := natsgo.Connect(natsURL)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Driver unable to connect to NATS server [%s]", natsURL))
	}

	var msgCollector *msgcheck.MsgCollector
	if msgDumpFile != "" || expectedMsgsFile != "" {
		msgCollector, err = msgcheck.NewMsgCollector(expectedMsgsFile)
		if err != nil {
			return nil, err
		}
	}

	d := BotRunner{
		logger:           driverLogger,
		playerLogger:     playerLogger,
		msgCollector:     msgCollector,
		msgDumpFile:      msgDumpFile,
		expectedMsgsFile: expectedMsgsFile,
		clubCode:         clubCode,
		botIsClubOwner:   clubCode == "",
		gameCode:         gameCode,
		botIsGameHost:    gameCode == "",
		config:           config,
		bots:             make([]*player.BotPlayer, 0),
		botsByName:       make(map[string]*player.BotPlayer),
		botsBySeat:       make(map[uint32]*player.BotPlayer),
		natsConn:         nc,
		currentHandNum:   0,
	}
	return &d, nil
}

// Terminate causes this BotRunner to eventually terminate, ending the ongoing game.
func (br *BotRunner) Terminate() {
	br.shouldTerminate = true
}

// Run sets up the game and joins the bots. Waits until the game is over.
func (br *BotRunner) Run() error {
	br.logger.Debug().Msgf("Config: %+v", br.config)

	resetDB := false
	if br.config.ResetDB {
		resetDB = true
	}

	if resetDB {
		url := fmt.Sprintf("%s/graphql", util.Env.GetAPIServerURL())
		gqlClient := graphql.NewClient(url)
		gqlHelper := gql.NewGQLHelper(gqlClient, 1000, "")
		gqlHelper.ResetDB()
	}
	// Create the player bots based on the setup script.
	for i, botConfig := range br.config.Setup.Players {
		b, err := player.NewBotPlayer(player.Config{
			Name:               botConfig.Name,
			DeviceID:           botConfig.DeviceID,
			Email:              botConfig.Email,
			Password:           botConfig.Password,
			IsHost:             (i == 0) && br.botIsGameHost, // First bot is the game host.
			IsHuman:            !botConfig.Bot,
			BotActionPauseTime: botConfig.BotActionPauseTime,
			APIServerURL:       util.Env.GetAPIServerURL(),
			NatsURL:            util.Env.GetNatsURL(),
			GQLTimeoutSec:      300,
			Script:             br.config,
		}, br.playerLogger, br.msgCollector)
		if err != nil {
			return errors.Wrap(err, "Unable to create a new bot")
		}
		br.bots = append(br.bots, b)
		br.botsByName[botConfig.Name] = b
	}

	// Create the observer bot. The observer will always be there
	// regardless of what script you run.
	b, err := player.NewBotPlayer(player.Config{
		Name:               "observer",
		DeviceID:           "e31c619f-a955-4f7b-985a-652992e01a7f",
		Email:              "observer@gmail.com",
		Password:           "mypassword",
		IsObserver:         true,
		BotActionPauseTime: 0,
		APIServerURL:       util.Env.GetAPIServerURL(),
		NatsURL:            util.Env.GetNatsURL(),
		GQLTimeoutSec:      30,
		Script:             br.config,
	}, br.playerLogger, br.msgCollector)
	if err != nil {
		return errors.Wrap(err, "Unable to create observer bot")
	}
	br.observerBot = b

	// Register bots to the poker service.
	for _, b := range append(br.bots, br.observerBot) {
		err := b.Register()
		if err != nil {
			return err
		}
	}

	if br.clubCode != "" {
		br.logger.Info().Msgf("Using an existing club [%s]", br.clubCode)
	} else {
		// if there is a club with the same name, just use the club-code
		clubCode, err := br.bots[0].GetClubCode(br.config.Setup.Club.Name)
		if clubCode == "" {
			// First bot creates the club. First bot is always the club owner. It is also responsible for
			// starting the game once all players are ready.
			clubCode, err = br.bots[0].CreateClub(br.config.Setup.Club.Name, br.config.Setup.Club.Description)
			if err != nil {
				return err
			}
		}
		br.clubCode = clubCode
		// create rewards for the club
		if len(br.config.Setup.Club.Rewards) > 0 {
			for _, reward := range br.config.Setup.Club.Rewards {
				_, err = br.bots[0].CreateClubReward(clubCode, reward.Name, reward.Type, reward.Schedule, reward.Amount)
			}
		}
	}

	// The bots apply for the club membership.
	botsToApplyClub := br.bots
	if br.botIsClubOwner {
		br.bots[0].SetClubCode(br.clubCode)
		// The owner bot does not need to apply to its own club.
		botsToApplyClub = br.bots[1:]
	}
	botsToApplyClub = append(botsToApplyClub, br.observerBot)
	for _, b := range botsToApplyClub {
		memberStatus, err := b.GetClubMemberStatus(br.clubCode)
		memberStatusStr := game.ClubMemberStatus_name[int32(memberStatus)]
		if memberStatusStr == "ACTIVE" {
			// This bot is already a member of the club.
			b.SetClubCode(br.clubCode)
			continue
		}
		// Submit the join request.
		err = b.JoinClub(br.clubCode)
		if err != nil {
			return err
		}
	}

	if br.botIsClubOwner {
		// The club owner bot approves the other bots to join the club.
		err = br.bots[0].ApproveClubMembers()
		if err != nil {
			return err
		}
	}

	// If the club's not owned by the bot, then we might need to wait for a human player to approve the bots.
	// Check if all bots are approved to the club. Wait if necessary.
	botsApprovedToClub := make([]*player.BotPlayer, 0)
	for waitAttempts := 0; len(botsApprovedToClub) != len(botsToApplyClub); waitAttempts++ {
		botsApprovedToClub = botsApprovedToClub[:0]
		for _, bot := range botsToApplyClub {
			memberStatus, err := bot.GetClubMemberStatus(br.clubCode)
			if err != nil {
				return err
			}
			memberStatusStr := game.ClubMemberStatus_name[int32(memberStatus)]
			if memberStatusStr == "ACTIVE" {
				botsApprovedToClub = append(botsApprovedToClub, bot)
			}
		}

		if len(botsApprovedToClub) != len(botsToApplyClub) {
			if waitAttempts%3 == 0 {
				botNamesNotApproved := br.getBotNamesDiff(botsToApplyClub, botsApprovedToClub)
				br.logger.Info().Msgf("Waiting for bots %v to be approved to the club [%s]", botNamesNotApproved, br.clubCode)
			}
			time.Sleep(2000 * time.Millisecond)
		}
	}

	rewardIds := make([]uint32, 0)
	if br.config.Setup.Game.Rewards != "" {
		// rewards can be listed with comma delimited string
		rewardID := br.bots[0].RewardsNameToId[br.config.Setup.Game.Rewards]
		rewardIds = append(rewardIds, rewardID)
	}

	gameTitle := br.config.Setup.Game.Title
	if br.config.GameTitle != "" {
		gameTitle = br.config.GameTitle
	}
	if br.gameCode == "" {
		// First bot creates the game.
		gameCode, err := br.bots[0].CreateGame(game.GameCreateOpt{
			Title:              gameTitle,
			GameType:           br.config.Setup.Game.GameType,
			SmallBlind:         br.config.Setup.Game.SmallBlind,
			BigBlind:           br.config.Setup.Game.BigBlind,
			UtgStraddleAllowed: br.config.Setup.Game.UtgStraddleAllowed,
			StraddleBet:        br.config.Setup.Game.StraddleBet,
			MinPlayers:         br.config.Setup.Game.MinPlayers,
			MaxPlayers:         br.config.Setup.Game.MaxPlayers,
			GameLength:         br.config.Setup.Game.GameLength,
			BuyInApproval:      br.config.Setup.Game.BuyInApproval,
			RakePercentage:     br.config.Setup.Game.RakePercentage,
			RakeCap:            br.config.Setup.Game.RakeCap,
			BuyInMin:           br.config.Setup.Game.BuyInMin,
			BuyInMax:           br.config.Setup.Game.BuyInMax,
			ActionTime:         br.config.Setup.Game.ActionTime,
			RewardIds:          rewardIds,
		})
		if err != nil {
			return err
		}
		br.gameCode = gameCode
	}

	// Let the observer bot start watching the game.
	br.observerBot.ObserveGame(br.gameCode)

	if br.botIsGameHost {
		for _, player := range br.config.Setup.Players {
			b := br.botsByName[player.Name]
			b.ObserveGame(br.gameCode)
		}

		// Let the players join the game (take a seat).
		for _, sitIn := range br.config.Setup.SitIn {
			b := br.botsByName[sitIn.PlayerName]
			br.botsBySeat[sitIn.SeatNo] = b
			if b.IsHuman() {
				// Let the tester join himself.
				continue
			}
			err = b.JoinGame(br.gameCode)
			if err != nil {
				return err
			}
		}

		// Check if all players are seated in. Wait if necessary.
		var playersJoined bool
		for waitAttempts := 0; !playersJoined; waitAttempts++ {
			playersJoined = true
			playersInSeat, err := br.bots[0].GetPlayersInSeat(br.gameCode)
			if err != nil {
				return err
			}
			for _, sitIn := range br.config.Setup.SitIn {
				if !br.isSitIn(sitIn.SeatNo, sitIn.PlayerName, playersInSeat) {
					playersJoined = false
					if waitAttempts%3 == 0 {
						br.logger.Info().Msgf("Waiting for player [%s] to join. Game Code: *** %s ***", sitIn.PlayerName, br.gameCode)
					}
				}
			}
			if !playersJoined {
				time.Sleep(2000 * time.Millisecond)
			}
		}

		// Check if all players have bought in. Wait if necessary.
		var playersBoughtIn bool
		for waitAttempts := 0; !playersBoughtIn; waitAttempts++ {
			playersBoughtIn = true
			playersInSeat, err := br.bots[0].GetPlayersInSeat(br.gameCode)
			if err != nil {
				return err
			}
			for _, buyIn := range br.config.Setup.BuyIn {
				if !br.isBoughtIn(buyIn.SeatNo, buyIn.BuyChips, playersInSeat) {
					playersBoughtIn = false
					if waitAttempts%3 == 0 {
						br.logger.Info().Msgf("Waiting for seat [%d] to buy in.", buyIn.SeatNo)
					}
				}
			}
			if !playersBoughtIn {
				time.Sleep(2000 * time.Millisecond)
			}
		}

		// Have the owner bot start the game.
		err = br.bots[0].StartGame(br.gameCode)
		if err != nil {
			return err
		}
	} else {
		// This is not a bot-created game. Ignore the script and just fill in all the empty seats.
		nextBotIdx := 0
		var gameInfo *game.GameInfo
		for nextBotIdx < len(br.bots) {
			gi, err := br.bots[0].GetGameInfo(br.gameCode)
			if err != nil {
				br.logger.Error().Msgf("Unable to get game info: %s", err)
				time.Sleep(1000 * time.Second)
				continue
			}
			gameInfo = &gi
			if len(gi.SeatInfo.AvailableSeats) == 0 {
				br.logger.Info().Msg("All seats are filled.")
				break
			}
			err = br.bots[nextBotIdx].JoinUnscriptedGame(br.gameCode)
			if err != nil {
				br.logger.Error().Msgf("Bot %d unable to join game [%s]: %s", nextBotIdx, br.gameCode, err)
				time.Sleep(1000 * time.Second)
				continue
			}
			time.Sleep(util.GetRandomMilliseconds(200, 500))
			nextBotIdx++
		}

		if gameInfo != nil {
			for _, seatInfo := range gameInfo.SeatInfo.PlayersInSeats {
				for _, bot := range br.bots {
					if seatInfo.SeatNo == bot.GetSeatNo() {
						bot.SetBalance(seatInfo.Stack)
					}
				}
			}
		}
	}

	// Wait till the game is over.
	requestedEndGame := false
	for !br.areBotsFinished() && !br.anyBotError() {
		if br.shouldTerminate && br.botIsGameHost && !requestedEndGame {
			err := br.bots[0].RequestEndGame(br.gameCode)
			if err != nil {
				br.logger.Error().Msgf("Error [%s] while requesting to end game [%s]", err, br.gameCode)
			} else {
				requestedEndGame = true
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	if br.anyBotError() {
		br.logBotErrors()
	}

	if br.msgDumpFile != "" {
		fmt.Printf("Dumping collected game/hand messages to %s\n", br.msgDumpFile)
		jsonStr, err := br.msgCollector.ToPrettyJSONString()
		if err != nil {
			return err
		}
		ioutil.WriteFile(br.msgDumpFile, []byte(jsonStr), 0644)
	}

	if br.expectedMsgsFile != "" {
		fmt.Println("Verifying received game/hand messages against the expected messages.")
		err := br.msgCollector.Verify()
		if err != nil {
			return errors.Wrapf(err, "Messages verification failed. Check the message dump file (%s) and the expected messages file (%s)", br.msgDumpFile, br.expectedMsgsFile)
		}
	}

	return nil
}

func (br *BotRunner) getBotNamesDiff(allBots []*player.BotPlayer, compareBots []*player.BotPlayer) []string {
	compareNames := make([]string, 0)
	for _, bot := range compareBots {
		compareNames = append(compareNames, bot.GetName())
	}
	diffNames := make([]string, 0)
	for _, bot := range allBots {
		if !util.ContainsString(compareNames, bot.GetName()) {
			diffNames = append(diffNames, bot.GetName())
		}
	}
	return diffNames
}

func (br *BotRunner) isSitIn(seatNo uint32, playerName string, playersInSeat []game.SeatInfo) bool {
	for _, p := range playersInSeat {
		if p.SeatNo == seatNo {
			if p.Name == playerName {
				return true
			}
			br.logger.Warn().Msgf("Seat [%d] is expected to be taken by [%s] but is already taken by another player [%s]", seatNo, playerName, p.Name)
			return false
		}
	}
	return false
}

func (br *BotRunner) isBoughtIn(seatNo uint32, numChips float32, playersInSeat []game.SeatInfo) bool {
	for _, p := range playersInSeat {
		if p.SeatNo == seatNo {
			if p.BuyIn == numChips {
				return true
			}
			if p.BuyIn != 0 {
				br.logger.Warn().Msgf("Seat [%d] expected to buy in [%f] chips, but bought in [%f] instead", seatNo, numChips, p.BuyIn)
			}
		}
	}
	return false
}

func (br *BotRunner) areBotsFinished() bool {
	for _, b := range br.bots {
		if b.IsHuman() {
			continue
		}
		if !b.IsSeated() {
			continue
		}
		if !b.IsGameOver() {
			return false
		}
	}
	return true
}

func (br *BotRunner) anyBotError() bool {
	for _, b := range br.bots {
		if b.IsErrorState() {
			return true
		}
	}
	return false
}

func (br *BotRunner) logBotErrors() {
	for _, b := range br.bots {
		if b.IsErrorState() {
			br.logger.Error().Msgf("Bot is in error state. Bot error message: %s", b.GetErrorMsg())
		}
	}
}
