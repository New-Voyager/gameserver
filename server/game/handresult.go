package game

import (
	"fmt"
	"math"

	"google.golang.org/protobuf/encoding/protojson"
	"voyager.com/server/poker"
)

type HandResultProcessor struct {
	handState         *HandState
	rewardTrackingIds []uint32
	evaluator         HandEvaluator
	hiLoGame          bool
}

func NewHandResultProcessor(handState *HandState, maxSeats uint32, rewardTrackingIds []uint32) *HandResultProcessor {
	var evaluator HandEvaluator
	includeHighHand := rewardTrackingIds != nil
	hiLoGame := false
	if handState.GameType == GameType_HOLDEM {
		evaluator = NewHoldemWinnerEvaluate(handState, includeHighHand, maxSeats)
	} else if handState.GameType == GameType_PLO || handState.GameType == GameType_FIVE_CARD_PLO {
		evaluator = NewPloWinnerEvaluate(handState, includeHighHand, false, maxSeats)
	} else if handState.GameType == GameType_PLO_HILO || handState.GameType == GameType_FIVE_CARD_PLO_HILO {
		evaluator = NewPloWinnerEvaluate(handState, includeHighHand, true, maxSeats)
		hiLoGame = true
	}
	return &HandResultProcessor{
		handState:         handState,
		rewardTrackingIds: rewardTrackingIds,
		evaluator:         evaluator,
		hiLoGame:          hiLoGame,
	}
}

func (hr *HandResultProcessor) getWinners() map[uint32]*PotWinners {
	return hr.evaluator.GetWinners()
}

func (hr *HandResultProcessor) getBoard2Winners() map[uint32]*PotWinners {
	return hr.evaluator.GetBoard2Winners()
}

func (hr *HandResultProcessor) getPlayerBalance(playerID uint64) *HandPlayerBalance {
	balanceBefore := float32(0)
	balanceAfter := float32(0)
	for _, playerBalance := range hr.handState.BalanceBeforeHand {
		if playerID == playerBalance.PlayerId {
			balanceBefore = playerBalance.Balance
			break
		}
	}

	for _, playerBalance := range hr.handState.BalanceAfterHand {
		if playerID == playerBalance.PlayerId {
			balanceAfter = playerBalance.Balance
			break
		}
	}
	return &HandPlayerBalance{Before: balanceBefore, After: balanceAfter}
}

func (hr *HandResultProcessor) populateCommunityCards(handResult *HandResult) {
	handResult.Turn = uint32(hr.handState.BoardCards[3])
	handResult.River = uint32(hr.handState.BoardCards[4])
	if hr.handState.BoardCards != nil {
		handResult.BoardCards = make([]uint32, len(hr.handState.BoardCards))
		for i, card := range hr.handState.BoardCards {
			handResult.BoardCards[i] = uint32(card)
		}
	}

	if hr.handState.BoardCards_2 != nil {
		handResult.BoardCards_2 = make([]uint32, len(hr.handState.BoardCards_2))
		for i, card := range hr.handState.BoardCards_2 {
			handResult.BoardCards_2[i] = uint32(card)
		}
	}

	handResult.Flop = make([]uint32, 3)
	for i, card := range hr.handState.BoardCards[:3] {
		handResult.Flop[i] = uint32(card)
	}
}

func (hr *HandResultProcessor) getResult(db bool) *HandResult {
	var bestSeatHands map[uint32]*EvaluatedCards
	var highHands map[uint32]*EvaluatedCards
	if hr.handState.PotWinners == nil {
		hr.evaluator.Evaluate()
		// update winners in hand state
		// this is also the method that calcualtes rake, balance etc
		hr.handState.setWinners(hr.getWinners(), false)
		if hr.handState.RunItTwiceConfirmed {
			hr.handState.setWinners(hr.getBoard2Winners(), true)
		}
	}

	// determine winners who went to showdown
	handState := hr.handState
	for _, winners := range handState.PotWinners {
		// we are going to walk through each winner and identify who won at showdown
		for _, winner := range winners.GetHiWinners() {
			playerID := handState.ActiveSeats[winner.SeatNo]
			if handState.PlayerStats[playerID].WentToShowdown {
				handState.PlayerStats[playerID].WonChipsAtShowdown = true
			}
			if handState.PlayerStats[playerID].Headsup {
				handState.PlayerStats[playerID].WonHeadsup = true
			}
		}

		for _, winner := range winners.GetLowWinners() {
			playerID := handState.ActiveSeats[winner.SeatNo]
			if handState.PlayerStats[playerID].WentToShowdown {
				handState.PlayerStats[playerID].WonChipsAtShowdown = true
			}

			if handState.PlayerStats[playerID].Headsup {
				handState.PlayerStats[playerID].WonHeadsup = true
			}
		}
	}

	// we want to evaulate the hands again for the high hand if the remaining player may have the high hand
	if (hr.handState.BoardCards != nil && hr.rewardTrackingIds != nil && len(hr.rewardTrackingIds) > 0) ||
		hr.handState.HandCompletedAt == HandStatus_SHOW_DOWN {
		bestSeatHands = hr.evaluator.GetBestPlayerCards()
		highHands = hr.evaluator.GetHighHandCards()
		// fmt.Printf("\n\n================================================================\n\n")
		// for seatNo, hand := range bestSeatHands {
		// 	if highHand, ok := highHands[seatNo]; ok {
		// 		fmt.Printf("Seat: %d, Cards:%+v, Str: %s Rank: %d, rankStr: %s, hhHand: %s rank: %d rankStr: %s\n",
		// 			seatNo,
		// 			hand.cards,
		// 			poker.CardsToString(hand.cards), hand.rank, poker.RankString(hand.rank),
		// 			poker.CardToString(highHand.cards), highHand.rank, poker.RankString((highHand.rank)))
		// 	} else {
		// 		fmt.Printf("Seat: %d, Cards:%+v, Str: %s Rank: %d, rankStr: %s\n",
		// 			seatNo,
		// 			hand.cards,
		// 			poker.CardsToString(hand.cards), hand.rank, poker.RankString(hand.rank))
		// 	}
		// }
		// fmt.Printf("\n\n================================================================\n\n")
	}
	handResult := &HandResult{
		GameId:     hr.handState.GameId,
		HandNum:    hr.handState.HandNum,
		GameType:   hr.handState.GameType,
		RunItTwice: hr.handState.RunItTwiceConfirmed,
	}

	// update stats in the result
	handResult.PlayerStats = hr.handState.PlayerStats
	handResult.HandStats = hr.handState.HandStats

	// get hand log
	handResult.HandLog = hr.handState.getLog()

	// record pots and seats associated with pots
	seatsAndPots := make([]*SeatsInPots, 0)
	h := hr.handState
	if h.Pots != nil && len(h.Pots) > 0 {
		for _, pot := range h.Pots {
			if len(pot.Seats) > 0 {
				seats := make([]uint32, len(pot.Seats))
				for i, seat := range pot.Seats {
					seats[i] = seat
				}
				seatPot := &SeatsInPots{
					Seats: seats,
					Pot:   pot.Pot,
				}
				seatsAndPots = append(seatsAndPots, seatPot)
			}
		}
	}
	handResult.HandLog.SeatsPotsShowdown = seatsAndPots

	handResult.RewardTrackingIds = hr.rewardTrackingIds
	hr.populateCommunityCards(handResult)

	handResult.Players = make(map[uint32]*PlayerInfo, 0)

	// populate players section
	for seatNo, playerID := range hr.handState.GetPlayersInSeats() {

		// no player in the seat
		if playerID == 0 {
			continue
		}
		playerState, _ := hr.handState.GetPlayersState()[playerID]

		// determine whether the player has folded
		playerFolded := false
		if hr.handState.ActiveSeats[seatNo] == 0 {
			playerFolded = true
		} else {
			playerState.Round = hr.handState.HandCompletedAt
		}

		// calculate high rank only the player hasn't folded
		rank := uint32(0xFFFFFFFF)
		highHandRank := uint32(0xFFFFFFFF)
		var bestCards []uint32
		var highHandBestCards []uint32

		cards := hr.handState.PlayersCards[uint32(seatNo)]
		playerCards := make([]uint32, len(cards))
		for i, card := range cards {
			playerCards[i] = uint32(card)
		}
		if !playerFolded {
			var evaluatedCards *EvaluatedCards
			if bestSeatHands != nil {
				evaluatedCards = bestSeatHands[uint32(seatNo)]
				if evaluatedCards != nil {
					rank = uint32(evaluatedCards.rank)
				}
			}

			if evaluatedCards != nil {
				bestCards = make([]uint32, len(evaluatedCards.cards))
				for i, card := range evaluatedCards.cards {
					bestCards[i] = uint32(card)
				}
			}
			if highHands != nil {
				if highHands[uint32(seatNo)] != nil {
					highHandRank = uint32(highHands[uint32(seatNo)].rank)
					highHandBestCards = highHands[uint32(seatNo)].GetCards()
				}
			}
		}

		playerInfo := &PlayerInfo{
			Id:          playerID,
			PlayedUntil: playerState.Round,
			Balance:     hr.getPlayerBalance(playerID),
		}

		if !playerFolded {
			// he won
			playerInfo.Received = playerState.PlayerReceived
		}

		if !playerFolded || db {
			// player is active or the result is stored in database
			playerInfo.Cards = playerCards
			playerInfo.BestCards = bestCards
			playerInfo.Rank = rank
			playerInfo.HhCards = highHandBestCards
			playerInfo.HhRank = highHandRank
		}

		if rakePaid, ok := hr.handState.RakePaid[playerID]; ok {
			playerInfo.RakePaid = rakePaid
		}
		handResult.RakeCollected = hr.handState.RakeCollected
		handResult.Players[uint32(seatNo)] = playerInfo
	}

	return handResult
}

func (hr *HandResultProcessor) evaluateRank() {
	hs := hr.handState

	// for each board, calculate rank for active players
	for _, board := range hs.Boards {
		board.PlayerRank = make(map[uint32]*BoardCardRank)
		// first calculate rank for each active seats
		for seatNoIdx, playerId := range hr.handState.ActiveSeats {
			if playerId == 0 {
				continue
			}
			seatNo := uint32(seatNoIdx)
			playersCards := hs.PlayersCards[seatNo]
			boardCards := poker.FromUint32ByteCards(board.Cards)
			eval := hr.evaluator.Evaluate2(playersCards, boardCards)
			lowFound := len(eval.locards) > 0
			hiCards := poker.ByteCardsToUint32Cards(eval.cards)
			board.PlayerRank[seatNo] = &BoardCardRank{
				BoardNo:  board.BoardNo,
				SeatNo:   seatNo,
				HiRank:   uint32(eval.rank),
				HiCards:  hiCards,
				LowFound: lowFound,
				LoRank:   uint32(eval.loRank),
				LoCards:  poker.ByteCardsToUint32Cards(eval.locards),
			}
		}
	}
}

func (hr *HandResultProcessor) determineHiLoRank(boardIndex int, seats []uint32) (int32, int32) {
	hs := hr.handState

	hiRank := int32(0x7FFFFFFF)
	loRank := int32(0x7FFFFFFF)
	first := true
	board := hs.Boards[boardIndex]
	// go through each board and determine winners for each board
	for seatNoIdx, playerId := range hs.ActiveSeats {
		if playerId == 0 {
			continue
		}
		seatNo := uint32(seatNoIdx)
		if first {
			hiRank = int32(board.PlayerRank[seatNo].HiRank)
		}
		if hr.hiLoGame && first {
			loRank = int32(board.PlayerRank[seatNo].LoRank)
		}
		if first {
			first = false
			continue
		}

		if int32(board.PlayerRank[seatNo].HiRank) < hiRank {
			hiRank = int32(board.PlayerRank[seatNo].HiRank)
		}

		if hr.hiLoGame && int32(board.PlayerRank[seatNo].LoRank) < loRank {
			loRank = int32(board.PlayerRank[seatNo].LoRank)
		}
	}

	return hiRank, loRank
}

func (hr *HandResultProcessor) determineWinners() *HandResultClient {
	hs := hr.handState
	// evaluate rank for all active players
	hr.evaluateRank()
	lowWinnerFound := false
	winningPlayers := make(map[uint32]bool)
	potWinners := make([]*PotWinnersV2, 0)
	// iterate through each pot
	for potNo, pot := range hs.Pots {
		potWinner := PotWinnersV2{
			PotNo:        uint32(potNo),
			Amount:       pot.Pot,
			BoardWinners: make([]*BoardWinner, 0),
		}

		// split the pot for each board
		potAmountForBoard := int(pot.Pot / float32(hs.NoOfBoards))
		remaining := pot.Pot - float32(potAmountForBoard*int(hs.NoOfBoards))
		boardPotAmounts := make([]float32, hs.NoOfBoards)
		for i := 0; i < int(hs.NoOfBoards); i++ {
			boardPotAmounts[i] = float32(potAmountForBoard)
			if remaining > 0 {
				boardPotAmounts[i]++
				remaining--
			}
		}

		// we calculate how much chips go to each board from this pot
		for i := 0; i < int(hs.NoOfBoards); i++ {
			boardPot := boardPotAmounts[i]
			board := hs.Boards[i]
			boardWinner := BoardWinner{
				BoardNo: board.BoardNo,
				Amount:  boardPot,
			}
			// determined winning ranks in this board
			hiRank, loRank := hr.determineHiLoRank(i, pot.Seats)
			boardWinner.HiRankText = poker.RankString(hiRank)

			hiWinners := make(map[uint32]*Winner)
			loWinners := make(map[uint32]*Winner)
			// determine winners
			for _, seatNo := range pot.Seats {

				// if this player is not active, ignore the player
				if hs.ActiveSeats[seatNo] == 0 {
					continue
				}

				if board.PlayerRank[seatNo].HiRank == uint32(hiRank) {
					winningPlayers[seatNo] = true
					hiWinners[seatNo] = &Winner{
						SeatNo: seatNo,
						Amount: 0,
					}
				}
				if hr.hiLoGame && loRank != 0x7FFFFFFF {
					if board.PlayerRank[seatNo].LoRank == uint32(loRank) {
						lowWinnerFound = true
						winningPlayers[seatNo] = true
						loWinners[seatNo] = &Winner{
							SeatNo: seatNo,
							Amount: 0,
						}
					}
				}
			}
			boardWinner.HiWinners = hiWinners
			boardWinner.LowWinners = loWinners

			hiWinnerPotAmount := boardPot
			loWinnerPotAmount := float32(0.0)
			if len(loWinners) > 0 {
				hiWinnerPotAmount = float32(int(boardPot / 2))
				if int(boardPot)%2 > 0 {
					hiWinnerPotAmount++
				}
				loWinnerPotAmount = boardPot - float32(hiWinnerPotAmount)
			}

			hiWinnerSplitPot := int(float32(hiWinnerPotAmount / float32(len(hiWinners))))
			remaining := hiWinnerPotAmount - float32(hiWinnerSplitPot*len(hiWinners))
			for _, hiWinner := range hiWinners {
				hiWinner.Amount = float32(hiWinnerSplitPot)
				if remaining > 0 {
					hiWinner.Amount++
					remaining--
				}
			}

			if len(loWinners) > 0 {
				loWinnerSplitPot := int(float32(loWinnerPotAmount / float32(len(loWinners))))
				remaining := loWinnerPotAmount - float32(loWinnerSplitPot*len(loWinners))
				for _, loWinner := range loWinners {
					loWinner.Amount = float32(loWinnerSplitPot)
					if remaining > 0 {
						loWinner.Amount++
						remaining--
					}
				}
			}

			// add to board winners
			potWinner.BoardWinners = append(potWinner.BoardWinners, &boardWinner)
		}
		potWinners = append(potWinners, &potWinner)
	}
	marshaller := protojson.MarshalOptions{
		EmitUnpopulated: true,
	}

	seats := make([]uint32, 0)
	for seatNo, playerID := range hs.ActiveSeats {
		if playerID == 0 {
			continue
		}
		seats = append(seats, uint32(seatNo))
	}

	// calculate rake and player balance
	playerInfo := hr.calcRakeAndBalance(hs, potWinners)

	scoop := false
	// if one players win multiple boards or both hi/lo cards, it is scoop
	if hs.NoOfBoards >= 2 {
		if len(winningPlayers) == 1 {
			scoop = true
		}
	} else {
		if lowWinnerFound {
			if len(winningPlayers) == 1 {
				scoop = true
			}
		}
	}

	// updates stats (player won chips at showdown)
	if hs.CurrentState == HandStatus_SHOW_DOWN {
		for seatNo, playerID := range hs.ActiveSeats {
			if playerID == 0 {
				continue
			}
			if winningPlayer, ok := winningPlayers[uint32(seatNo)]; ok {
				if winningPlayer {
					hs.PlayerStats[playerID].WonChipsAtShowdown = true
				}
			}
		}
	}

	result := &HandResultClient{
		ActiveSeats:   seats,
		WonAt:         hs.getLog().GetWonAt(),
		PotWinners:    potWinners,
		Boards:        hs.Boards,
		PauseTimeSecs: 5000,
		PlayerInfo:    playerInfo,
		Scoop:         scoop,
		HandNum:       hs.HandNum,
	}
	jsonb, _ := marshaller.Marshal(result)
	fmt.Printf("\n\n\n")
	fmt.Printf(string(jsonb))
	fmt.Printf("\n\n\n")
	return result
}

func (hr *HandResultProcessor) calcRakeAndBalance(hs *HandState, potWinners []*PotWinnersV2) map[uint32]*PlayerHandInfo {
	playerStack := make(map[uint64]float32)
	playerReceived := make(map[uint32]float32)

	for seatNoIdx, playerID := range hs.PlayersInSeats {
		if playerID == 0 {
			continue
		}
		playerStack[playerID] = hs.PlayersState[playerID].Stack
		playerReceived[uint32(seatNoIdx)] = 0
	}
	totalPot := float32(0)
	// update player balance
	for _, pot := range potWinners {
		totalPot += pot.Amount
		for _, board := range pot.BoardWinners {
			for _, handWinner := range board.HiWinners {
				seatNo := handWinner.SeatNo
				playerID := hs.PlayersInSeats[seatNo]
				playerStack[playerID] = playerStack[playerID] + handWinner.Amount
				playerReceived[seatNo] = playerReceived[seatNo] + handWinner.Amount
			}

			for _, handWinner := range board.LowWinners {
				seatNo := handWinner.SeatNo
				playerID := hs.PlayersInSeats[seatNo]
				playerStack[playerID] = playerStack[playerID] + handWinner.Amount
				playerReceived[seatNo] = playerReceived[seatNo] + handWinner.Amount
			}
		}
	}
	rakePlayers := make(map[uint64]float32, 0)

	if hs.RakePercentage > 0 {
		// calculate rake from the total pot
		rake := float32(totalPot * (hs.RakePercentage / 100))
		rake = float32(math.Floor(float64(rake)))
		if rake <= 0 {
			rake = 1.0
		}
		if hs.RakeCap != 0 {
			if rake > hs.RakeCap {
				rake = hs.RakeCap
			}
		}

		rakePaid := make(map[uint32]float32, 0)
		for seatNo, playerID := range hs.PlayersInSeats {
			if playerID == 0 {
				continue
			}
			rakePaid[uint32(seatNo)] = 0
		}

		// rake from player who won money
		rakeFromPlayer := float32(0.0)
		if int(rake) > 0 {
			winnerCount := 0
			for _, pot := range potWinners {
				for _, board := range pot.BoardWinners {
					winnerCount = winnerCount + len(board.HiWinners)
					winnerCount = winnerCount + len(board.LowWinners)
				}
			}
			rakeFromPlayer = float32(int(rake / float32(winnerCount)))
			if rakeFromPlayer == 0.0 {
				rakeFromPlayer = 1.0
			}
			totalRakeCollected := float32(0)
			for _, pot := range potWinners {
				for _, board := range pot.BoardWinners {
					for _, handWinner := range board.HiWinners {
						seatNo := handWinner.SeatNo
						handWinner.Amount -= rakeFromPlayer
						rakePaid[seatNo] += rakeFromPlayer
						totalRakeCollected += rakeFromPlayer
						if totalRakeCollected >= rake {
							break
						}
					}
					if totalRakeCollected >= rake {
						break
					}
					for _, handWinner := range board.LowWinners {
						seatNo := handWinner.SeatNo
						handWinner.Amount -= rakeFromPlayer
						rakePaid[seatNo] += rakeFromPlayer
						totalRakeCollected += rakeFromPlayer
						if totalRakeCollected >= rake {
							break
						}
					}
					if totalRakeCollected >= rake {
						break
					}
				}
			}

			for seatNo, rakeAmount := range rakePaid {
				playerID := hs.PlayersInSeats[seatNo]
				if rakeAmount > 0 {
					playerStack[playerID] = playerStack[playerID] - rakeAmount
					playerReceived[seatNo] = playerReceived[seatNo] - rakeAmount
					rakePlayers[playerID] = rakeAmount
				}
			}
			hs.RakePaid = rakePlayers
			hs.RakeCollected = totalRakeCollected
		}
	}

	players := make(map[uint32]*PlayerHandInfo)
	for seatNoIdx, playerID := range hs.PlayersInSeats {
		if playerID == 0 {
			continue
		}
		seatNo := uint32(seatNoIdx)
		playerState := hs.PlayersState[playerID]
		players[seatNo] = &PlayerHandInfo{
			Id:          playerID,
			PlayedUntil: playerState.Round,
			Cards:       poker.ByteCardsToUint32Cards(hs.PlayersCards[seatNo]),
		}
	}

	// also populate current balance of the players in the table
	for seatNo, player := range hs.PlayersInSeats {
		if player == 0 {
			continue
		}
		state := hs.PlayersState[player]

		before := float32(0.0)
		for _, playerBalance := range hs.BalanceBeforeHand {
			if playerBalance.SeatNo == uint32(seatNo) {
				before = playerBalance.Balance
				break
			}
		}
		playerBalance := &HandPlayerBalance{
			Before: before,
		}
		if currentBal, ok := playerStack[player]; ok {
			// winner
			playerBalance = &HandPlayerBalance{
				Before: before,
				After:  currentBal,
			}
		} else {
			// other players
			playerBalance = &HandPlayerBalance{
				Before: before,
				After:  state.Stack,
			}
		}
		rakePaidAmount := float32(0.0)
		if rake, ok := rakePlayers[player]; ok {
			rakePaidAmount = rake
		}

		players[uint32(seatNo)] = &PlayerHandInfo{
			Id:          player,
			PlayedUntil: state.Round,
			Cards:       poker.ByteCardsToUint32Cards(hs.PlayersCards[uint32(seatNo)]),
			Balance:     playerBalance,
			Received:    playerBalance.After - playerBalance.Before,
			RakePaid:    rakePaidAmount,
		}
	}
	return players
}
