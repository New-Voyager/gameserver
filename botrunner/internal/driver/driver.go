// TOOD: Need some way to mark the human-controlled bots (IsHuman) in the script yaml.

package driver

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jmoiron/sqlx"
	natsgo "github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"voyager.com/botrunner/internal/game"
	"voyager.com/botrunner/internal/player"
	"voyager.com/botrunner/internal/util"
	"voyager.com/gamescript"
)

// BotRunner is the main driver object that sets up the bots for a game.
type BotRunner struct {
	logger          *zerolog.Logger
	playerLogger    *zerolog.Logger
	clubCode        string
	botIsClubOwner  bool
	players         *gamescript.Players
	script          *gamescript.Script
	gameCode        string
	botIsGameHost   bool
	currentHandNum  uint32
	bots            []*player.BotPlayer
	observerBot     *player.BotPlayer
	botsByName      map[string]*player.BotPlayer
	botsBySeat      map[uint32]*player.BotPlayer
	observerBots    map[string]*player.BotPlayer // these players are observing the game and waiting in the waitlist
	apiServerURL    string
	natsConn        *natsgo.Conn
	shouldTerminate bool
	resetDB         bool
	playerGame      bool
}

// NewBotRunner creates new instance of BotRunner.
func NewBotRunner(clubCode string, gameCode string, script *gamescript.Script, players *gamescript.Players, driverLogger *zerolog.Logger, playerLogger *zerolog.Logger, resetDB bool, playerGame bool) (*BotRunner, error) {
	natsURL := util.Env.GetNatsURL()
	nc, err := natsgo.Connect(natsURL)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Driver unable to connect to NATS server [%s]", natsURL))
	}
	if clubCode == "null" {
		clubCode = ""
	}

	d := BotRunner{
		logger:         driverLogger,
		playerLogger:   playerLogger,
		clubCode:       clubCode,
		botIsClubOwner: clubCode == "",
		gameCode:       gameCode,
		botIsGameHost:  gameCode == "",
		players:        players,
		script:         script,
		bots:           make([]*player.BotPlayer, 0),
		botsByName:     make(map[string]*player.BotPlayer),
		botsBySeat:     make(map[uint32]*player.BotPlayer),
		observerBots:   make(map[string]*player.BotPlayer),
		natsConn:       nc,
		currentHandNum: 0,
		resetDB:        resetDB,
		playerGame:     playerGame,
	}
	return &d, nil
}

// Terminate causes this BotRunner to eventually terminate, ending the ongoing game.
func (br *BotRunner) Terminate() {
	br.shouldTerminate = true
}

// Run sets up the game and joins the bots. Waits until the game is over.
func (br *BotRunner) Run() error {
	br.logger.Debug().Msgf("Players: %+v, Script: %+v", br.players, br.script)

	// Create the player bots based on the setup script.
	for i, playerConfig := range br.players.Players {
		bot, err := player.NewBotPlayer(player.Config{
			Name:               playerConfig.Name,
			DeviceID:           playerConfig.DeviceID,
			Email:              playerConfig.Email,
			Password:           playerConfig.Password,
			IsHost:             (i == 0) && br.botIsGameHost, // First bot is the game host.
			IsHuman:            br.script.Tester == playerConfig.Name,
			MinActionPauseTime: br.script.BotConfig.MinActionPauseTime,
			MaxActionPauseTime: br.script.BotConfig.MaxActionPauseTime,
			APIServerURL:       util.Env.GetAPIServerURL(),
			NatsURL:            util.Env.GetNatsURL(),
			GQLTimeoutSec:      10,
			Script:             br.script,
			Players:            br.players,
		}, br.playerLogger)
		if err != nil {
			return errors.Wrap(err, "Unable to create a new bot")
		}
		br.bots = append(br.bots, bot)
		br.botsByName[playerConfig.Name] = bot
	}

	// Create the observer bot. The observer will always be there
	// regardless of what script you run.
	b, err := player.NewBotPlayer(player.Config{
		Name:               "observer",
		DeviceID:           "e31c619f-a955-4f7b-985a-652992e01a7f",
		Email:              "observer@gmail.com",
		Password:           "mypassword",
		IsObserver:         true,
		MinActionPauseTime: 0,
		MaxActionPauseTime: 0,
		APIServerURL:       util.Env.GetAPIServerURL(),
		NatsURL:            util.Env.GetNatsURL(),
		GQLTimeoutSec:      10,
		Script:             br.script,
		Players:            br.players,
	}, br.playerLogger)
	if err != nil {
		return errors.Wrap(err, "Unable to create observer bot")
	}
	br.observerBot = b

	for _, bot := range br.bots {
		bot.SetBotsInGame(br.bots)
	}

	//br.resetDB = false
	if br.resetDB {
		err = br.observerBot.ResetDB()
		if err != nil {
			panic("Resetting database failed")
		}
	}
	// we need to set the server settings before the player is created
	if br.botIsGameHost {
		if br.script.ServerSettings != nil {
			br.observerBot.SetupServerSettings(br.script.ServerSettings)
		}
	}

	br.logger.Info().Msgf("Bots joining the club")
	// Register bots to the poker service.
	for _, b := range append(br.bots, br.observerBot) {
		// Try logging in first. The bot player might've already signed up from some other game.
		err := b.Login()
		if err == nil {
			continue
		}
		// This bot has never signed up. Go ahead and sign up.
		err = b.SignUp()
		if err != nil {
			return errors.Wrapf(err, "%s cannot sign up", b.GetName())
		}
	}
	rewardIds := make([]uint32, 0)

	if !br.playerGame {
		if br.clubCode != "" {
			br.logger.Info().Msgf("Using an existing club [%s]", br.clubCode)
		} else {
			// if there is a club with the same name, just use the club-code
			clubCode, err := br.bots[0].GetClubCode(br.script.Club.Name)
			if clubCode == "" {
				// First bot creates the club. First bot is always the club owner. It is also responsible for
				// starting the game once all players are ready.
				clubCode, err = br.bots[0].CreateClub(br.script.Club.Name, br.script.Club.Description)
				if err != nil {
					return err
				}
			}
			br.clubCode = clubCode
			// create rewards for the club
			if len(br.script.Club.Rewards) > 0 {
				for _, reward := range br.script.Club.Rewards {
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
		br.logger.Info().Msgf("Bots joined the club")

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
				time.Sleep(100 * time.Millisecond)
			}
		}

		if br.script.Game.Rewards != "" {
			// rewards can be listed with comma delimited string
			//rewardID := br.bots[0].RewardsNameToID[br.script.Game.Rewards]
			rewardID, err := br.bots[0].GetRewardID(br.clubCode, br.script.Game.Rewards)
			if err != nil {
				br.logger.Error().Msgf("Could not get reward info for %s", br.script.Game.Rewards)
			} else {
				rewardIds = append(rewardIds, rewardID)
			}
		}
	}
	br.logger.Info().Msgf("Bots joining the new game")

	gameTitle := br.script.Game.Title
	if br.gameCode == "" {
		// First bot creates the game.
		gameCode, err := br.bots[0].CreateGame(game.GameCreateOpt{
			Title:              gameTitle,
			GameType:           br.script.Game.GameType,
			SmallBlind:         br.script.Game.SmallBlind,
			BigBlind:           br.script.Game.BigBlind,
			UtgStraddleAllowed: br.script.Game.UtgStraddleAllowed,
			StraddleBet:        br.script.Game.StraddleBet,
			MinPlayers:         br.script.Game.MinPlayers,
			MaxPlayers:         br.script.Game.MaxPlayers,
			GameLength:         br.script.Game.GameLength,
			BuyInApproval:      br.script.Game.BuyInApproval,
			RakePercentage:     br.script.Game.RakePercentage,
			RakeCap:            br.script.Game.RakeCap,
			BuyInMin:           br.script.Game.BuyInMin,
			BuyInMax:           br.script.Game.BuyInMax,
			ActionTime:         br.script.Game.ActionTime,
			RewardIds:          rewardIds,
			RunItTwiceAllowed:  br.script.Game.RunItTwiceAllowed,
			MuckLosingHand:     br.script.Game.MuckLosingHand,
			RoeGames:           br.script.Game.RoeGames,
			DealerChoiceGames:  br.script.Game.DealerChoiceGames,
			HighHandTracked:    br.script.Game.HighHandTracked,
			AppCoinsNeeded:     br.script.Game.AppCoinsNeeded,
			IpCheck:            br.script.Game.IpCheck,
			GpsCheck:           br.script.Game.GpsCheck,
		})
		if err != nil {
			return err
		}
		br.gameCode = gameCode
	}
	br.logger.Info().Msgf("New game is created")

	// Let the observer bot start watching the game.
	br.observerBot.ObserveGame(br.gameCode)
	br.logger.Info().Msgf("Starting the game")
	allJoinedGame := false
	skipPlayers := make([]string, 0)
	if br.botIsGameHost {
		if br.script.ServerSettings != nil {
			br.observerBot.SetupServerSettings(br.script.ServerSettings)
		}

		// This is a bot-created game. Use the config script to sit the bots.
		for _, startingSeat := range br.script.StartingSeats {
			playerName := startingSeat.Player
			b := br.botsByName[playerName]

			if startingSeat.Seat != 0 {
				br.botsBySeat[startingSeat.Seat] = b
				if b.IsHuman() {
					// Let the tester join himself.
					continue
				}

				// set location if specified
				if startingSeat.IpAddress != nil {
					b.SetIPAddress(*startingSeat.IpAddress)
				}

				err = b.JoinGame(br.gameCode, startingSeat.Gps)
				if err != nil {
					if startingSeat.IgnoreError != nil {
						if !*startingSeat.IgnoreError {
							return err
						}
						skipPlayers = append(skipPlayers, startingSeat.Player)
						allJoinedGame = true
					} else {
						return err
					}
				}
			} else {
				// observers
			}
		}
		for _, observer := range br.script.Observers {
			playerName := observer.Player
			b := br.botsByName[playerName]
			b.ObserveGame(br.gameCode)
			br.observerBots[playerName] = b
			br.logger.Info().Msgf("Player [%s] is observing. Game Code: *** %s ***", playerName, br.gameCode)
		}

		// Check if all players are seated in. Wait if necessary.
		var playersJoined bool
		if !allJoinedGame {
			for waitAttempts := 0; !playersJoined; waitAttempts++ {
				playersJoined = true
				playersInSeat, err := br.bots[0].GetPlayersInSeat(br.gameCode)
				if err != nil {
					return err
				}
				for _, startingSeat := range br.script.StartingSeats {
					if !br.isSitIn(startingSeat.Seat, startingSeat.Player, playersInSeat) {
						playersJoined = false
						if waitAttempts%3 == 0 {
							br.logger.Info().Msgf("Waiting for player [%s] to join. Game Code: *** %s ***", startingSeat.Player, br.gameCode)
						}
					}
				}
				if !playersJoined {
					time.Sleep(500 * time.Millisecond)
				}
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
			for _, startingSeat := range br.script.StartingSeats {
				skipPlayer := false
				// skip players if the player didn't join the game
				if len(skipPlayers) > 0 {
					for _, player := range skipPlayers {
						if player == startingSeat.Player {
							skipPlayer = true
						}
					}
				}

				if !skipPlayer && !br.isBoughtIn(startingSeat.Seat, startingSeat.BuyIn, playersInSeat) {
					playersBoughtIn = false
					if waitAttempts%3 == 0 {
						br.logger.Info().Msgf("Waiting for seat [%d] to buy in.", startingSeat.Seat)
					}
				}
			}
			if !playersBoughtIn {
				time.Sleep(100 * time.Millisecond)
			}
		}
		br.logger.Info().Msgf("Bots joined the new game")

		br.logger.Info().Msgf("Starting the new game")
		// Have the owner bot start the game.
		if !br.script.Game.DontStart {
			// Have the owner bot start the game.
			err = br.bots[0].StartGame(br.gameCode)
			if err != nil {
				return err
			}

			// add the players who are in waitlist
			for _, observer := range br.script.Observers {
				playerName := observer.Player
				b := br.botsByName[playerName]
				if observer.Waitlist {
					b.JoinWaitlist(&observer)
					br.logger.Info().Msgf("Player [%s] is in waitlist. Game Code: *** %s ***", playerName, br.gameCode)
				}
			}
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
			// time.Sleep(util.GetRandomMilliseconds(200, 500))
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

	br.logger.Info().Msgf("Game started")

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

	if br.botIsGameHost {
		br.observerBot.ResetServerSettings()
	}

	br.logger.Info().Msg("Processing after-game assertions")
	err = br.processAfterGameAssertions()
	if err != nil {
		return errors.Wrap(err, "Error in after-game check")
	}

	if br.anyBotError() {
		errMsg := br.logBotErrors()
		if errMsg != "" {
			return fmt.Errorf(errMsg)
		}
	}

	// Verify game-server crashed as requested.
	err = br.verifyGameServerCrashLog()
	if err != nil {
		return err
	}

	return nil
}

func (br *BotRunner) processAfterGameAssertions() error {
	errMsgs := make([]string, 0)
	minExpectedHands := br.script.AfterGame.Verify.NumHandsPlayed.Gte
	maxExpectedHands := br.script.AfterGame.Verify.NumHandsPlayed.Lte
	totalHandsPlayed := br.observerBot.GetHandResult2().HandNum
	if minExpectedHands != nil {
		if totalHandsPlayed < *minExpectedHands {
			errMsgs = append(errMsgs, fmt.Sprintf("Total hands played: %d, Expected AT LEAST %d hands to have been played", totalHandsPlayed, *minExpectedHands))
		}
	}
	if maxExpectedHands != nil {
		if totalHandsPlayed > *maxExpectedHands {
			errMsgs = append(errMsgs, fmt.Sprintf("Total hands played: %d, Expected AT MOST %d hands to have been played", totalHandsPlayed, *maxExpectedHands))
		}
	}

	for _, verifyPrivateMessage := range br.script.AfterGame.Verify.PrivateMessages {
		playerName := verifyPrivateMessage.Player
		if bot, found := br.botsByName[playerName]; found {
			for _, verifyMessage := range verifyPrivateMessage.Messages {
				// verify message exists
				found := false
				for _, message := range bot.PrivateMessages {
					messageType := fmt.Sprintf("%v", message["type"])
					if messageType == verifyMessage.Type {
						found = true
						break
					}
				}

				if !found {
					// message is not found
					errMsgs = append(errMsgs, fmt.Sprintf("%s Message type: %s is not found in the private messages", playerName, verifyMessage.Type))
				}
			}
		}
	}

	for _, verifyGameMessage := range br.script.AfterGame.Verify.GameMessages {
		// verify message exists
		found := false
		var gameMessageVerified *gamescript.NonProtoMessage
		for _, gameMessage := range br.observerBot.GameMessages {
			gameMessageVerified = gameMessage
			if gameMessage.Verified {
				continue
			}

			if verifyGameMessage.Type == "NEW_HIGHHAND_WINNER" {
				// compare the winners
				if cmp.Equal(verifyGameMessage.Winners, gameMessage.Winners) {
					found = true
					break
				}
				continue
			}
			if verifyGameMessage.Type == gameMessage.Type &&
				verifyGameMessage.SubType == gameMessage.SubType {
				found = true
				break
			}
		}

		if !found {
			// message is not found
			errMsgs = append(errMsgs, fmt.Sprintf("Message type: %s subType: %s is not found in the private messages",
				verifyGameMessage.Type, verifyGameMessage.SubType))
		} else {
			if verifyGameMessage.Type == "TABLE_UPDATE" &&
				verifyGameMessage.SubType == "HostSeatChangeMove" {
				error := false
				if len(verifyGameMessage.SeatMoves) != len(gameMessageVerified.SeatMoves) {
					errMsgs = append(errMsgs, fmt.Sprintf("Incorrect number of seat moves "))
					error = true
				}
				if !error {
					for idx, expectedMove := range verifyGameMessage.SeatMoves {
						actualMove := gameMessageVerified.SeatMoves[idx]
						if expectedMove.Name != actualMove.Name ||
							expectedMove.OldSeatNo != actualMove.OldSeatNo ||
							expectedMove.NewSeatNo != actualMove.NewSeatNo ||
							expectedMove.OpenSeat != actualMove.OpenSeat {
							errMsgs = append(errMsgs, fmt.Sprintf("Incorrect data in seat moves"))
						}
					}
				}
				gameMessageVerified.Verified = true
			} else if verifyGameMessage.Type == "PLAYER_SEAT_CHANGE_PROMPT" {
				if verifyGameMessage.PlayerName != gameMessageVerified.PlayerName ||
					verifyGameMessage.OpenedSeat != gameMessageVerified.OpenedSeat {
					errMsgs = append(errMsgs, fmt.Sprintf("Invalid data in PLAYER_SEAT_CHANGE_PROMPT"))
				}
			} else if verifyGameMessage.Type == "PLAYER_SEAT_MOVE" {
				if verifyGameMessage.PlayerName != gameMessageVerified.PlayerName ||
					uint32(verifyGameMessage.OldSeatNo) != uint32(gameMessageVerified.OldSeatNo) ||
					verifyGameMessage.NewSeatNo != gameMessageVerified.NewSeatNo {
					errMsgs = append(errMsgs, fmt.Sprintf("Invalid data in PLAYER_SEAT_MOVE"))
				}
			}
		}
	}
	if len(errMsgs) > 0 {
		return fmt.Errorf(strings.Join(errMsgs, "\n"))
	}
	return nil
}

func (br *BotRunner) verifyGameServerCrashLog() error {
	var expectedCrashPoints []string
	for _, hand := range br.script.Hands {
		for _, pd := range hand.Setup.PreDeal {
			cp := pd.SetupServerCrash.CrashPoint
			if cp != "" {
				expectedCrashPoints = append(expectedCrashPoints, cp)
			}
		}
		rounds := []gamescript.BettingRound{hand.Preflop, hand.Flop, hand.Turn, hand.River}
		for _, round := range rounds {
			for _, seatAction := range round.SeatActions {
				for _, preaction := range seatAction.PreActions {
					cp := preaction.SetupServerCrash.CrashPoint
					if cp != "" {
						expectedCrashPoints = append(expectedCrashPoints, cp)
					}
				}
			}
		}
	}

	db := sqlx.MustConnect("postgres", util.Env.GetPostgresConnStr())
	defer db.Close()
	var crashPoints []string
	query := fmt.Sprintf("SELECT crash_point FROM crash_test WHERE game_code = '%s' ORDER BY \"createdAt\" ASC", br.gameCode)
	err := db.Select(&crashPoints, query)
	if err != nil {
		return errors.Wrapf(err, "Error from sqlx. Query: [%s]", query)
	}

	if len(expectedCrashPoints) > 0 {
		br.logger.Info().Msgf("Expected Crash Points: %v", expectedCrashPoints)
		br.logger.Info().Msgf("Actual Crash Points  : %v", crashPoints)
	}
	if !cmp.Equal(crashPoints, expectedCrashPoints) {
		return fmt.Errorf("Game server crash log does not match the expected. Crashed: %v, Expected: %v", crashPoints, expectedCrashPoints)
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

func (br *BotRunner) logBotErrors() string {
	var errMsg string
	for _, b := range br.bots {
		if b.IsErrorState() {
			msg := fmt.Sprintf("Bot %s is in error state. Bot error message: %s", b.GetName(), b.GetErrorMsg())
			br.logger.Error().Msgf(msg)
			errMsg = errMsg + "\n" + msg
		}
	}
	return errMsg
}
