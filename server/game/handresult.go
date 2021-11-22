package game

import (
	"math"
	"sort"

	"voyager.com/server/poker"
	"voyager.com/server/util"
)

type HandResultProcessor struct {
	handState         *HandState
	chipUnit          ChipUnit
	rewardTrackingIds []uint32
	evaluator         HandEvaluator
	hiLoGame          bool
}

func NewHandResultProcessor(handState *HandState, chipUnit ChipUnit, maxSeats uint32, rewardTrackingIds []uint32) *HandResultProcessor {
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
		chipUnit:          chipUnit,
		rewardTrackingIds: rewardTrackingIds,
		evaluator:         evaluator,
		hiLoGame:          hiLoGame,
	}
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
				HhCards:  poker.ByteCardsToUint32Cards(eval.hhCards),
				HhRank:   uint32(eval.hhRank),
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

		// if the seat is not in the pot, move to the next
		found := false
		for _, potSeat := range seats {
			if potSeat == seatNo {
				found = true
				break
			}
		}

		if !found {
			continue
		}

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
			SeatsInPots:  pot.Seats,
		}

		// split the pot for each board
		boardPotAmounts := make([]float64, hs.NoOfBoards)
		if hr.chipUnit == ChipUnit_CENT {
			var potAmountForBoard float64 = util.FloorDecimal(pot.Pot/float64(hs.NoOfBoards), 2)
			remaining := pot.Pot - potAmountForBoard*float64(hs.NoOfBoards)
			for i := 0; i < int(hs.NoOfBoards); i++ {
				boardPotAmounts[i] = potAmountForBoard
				if util.Greater(remaining, 0) {
					boardPotAmounts[i] += 0.01
					remaining -= 0.01
				}
				boardPotAmounts[i] = util.RoundDecimal(boardPotAmounts[i], 2)
			}
		} else {
			var potAmountForBoard int = int(pot.Pot / float64(hs.NoOfBoards))
			remaining := pot.Pot - float64(potAmountForBoard*int(hs.NoOfBoards))
			for i := 0; i < int(hs.NoOfBoards); i++ {
				boardPotAmounts[i] = float64(potAmountForBoard)
				if remaining > 0 {
					boardPotAmounts[i]++
					remaining--
				}
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

				player := hs.PlayersInSeats[seatNo]
				if board.PlayerRank[seatNo].HiRank == uint32(hiRank) {
					winningPlayers[seatNo] = true
					hiWinners[seatNo] = &Winner{
						SeatNo: seatNo,
						Amount: 0,
					}
					if hs.PlayerStats[player.PlayerId].Headsup {
						hs.PlayerStats[player.PlayerId].WonHeadsup = true
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
					if hs.PlayerStats[player.PlayerId].Headsup {
						hs.PlayerStats[player.PlayerId].WonHeadsup = true
					}
				}
			}
			boardWinner.HiWinners = hiWinners
			boardWinner.LowWinners = loWinners

			if hr.chipUnit == ChipUnit_CENT {
				hiWinnerPotAmount := boardPot
				loWinnerPotAmount := float64(0.0)
				if len(loWinners) > 0 {
					hiWinnerPotAmount = util.RoundDecimal(boardPot/2, 2)
					loWinnerPotAmount = boardPot - hiWinnerPotAmount
				}

				hiWinnerSplitPot := util.FloorDecimal(hiWinnerPotAmount/float64(len(hiWinners)), 2)
				remaining := hiWinnerPotAmount - hiWinnerSplitPot*float64(len(hiWinners))
				for _, hiWinner := range hiWinners {
					hiWinner.Amount = hiWinnerSplitPot
					if util.Greater(remaining, 0) {
						hiWinner.Amount += 0.01
						remaining -= 0.01
					}
					hiWinner.Amount = util.RoundDecimal(hiWinner.Amount, 2)
				}

				if len(loWinners) > 0 {
					loWinnerSplitPot := util.FloorDecimal(loWinnerPotAmount/float64(len(loWinners)), 2)
					remaining := loWinnerPotAmount - loWinnerSplitPot*float64(len(loWinners))
					for _, loWinner := range loWinners {
						loWinner.Amount = loWinnerSplitPot
						if util.Greater(remaining, 0) {
							loWinner.Amount += 0.01
							remaining -= 0.01
						}
						loWinner.Amount = util.RoundDecimal(loWinner.Amount, 2)
					}
				}
			} else {
				hiWinnerPotAmount := boardPot
				loWinnerPotAmount := float64(0.0)
				if len(loWinners) > 0 {
					hiWinnerPotAmount = float64(int(boardPot / 2))
					if int(boardPot)%2 > 0 {
						hiWinnerPotAmount++
					}
					loWinnerPotAmount = boardPot - float64(hiWinnerPotAmount)
				}

				hiWinnerSplitPot := int(float64(hiWinnerPotAmount / float64(len(hiWinners))))
				remaining := hiWinnerPotAmount - float64(hiWinnerSplitPot*len(hiWinners))
				for _, hiWinner := range hiWinners {
					hiWinner.Amount = float64(hiWinnerSplitPot)
					if remaining > 0 {
						hiWinner.Amount++
						remaining--
					}
				}

				if len(loWinners) > 0 {
					loWinnerSplitPot := int(float64(loWinnerPotAmount / float64(len(loWinners))))
					remaining := loWinnerPotAmount - float64(loWinnerSplitPot*len(loWinners))
					for _, loWinner := range loWinners {
						loWinner.Amount = float64(loWinnerSplitPot)
						if remaining > 0 {
							loWinner.Amount++
							remaining--
						}
					}
				}
			}

			// add to board winners
			potWinner.BoardWinners = append(potWinner.BoardWinners, &boardWinner)
		}
		potWinners = append(potWinners, &potWinner)
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
		PauseTimeSecs: hs.ResultPauseTime,
		PlayerInfo:    playerInfo,
		Scoop:         scoop,
		HandNum:       hs.HandNum,
		TipsCollected: hs.RakeCollected,
	}
	return result
}

func (hr *HandResultProcessor) adjustRake(hs *HandState, totalPot float64, winners []uint32, potWinners []*PotWinnersV2, playerStack map[uint64]float64, playerReceived map[uint32]float64) map[uint64]float64 {
	sort.Slice(winners, func(a, b int) bool { return winners[a] < winners[b] })

	rakePlayers := make(map[uint64]float64)

	// calculate rake from the total pot
	var rake float64 = totalPot * (hs.RakePercentage / 100)
	if hr.chipUnit == ChipUnit_CENT {
		rake = util.FloorDecimal(rake, 2)
		if rake < 0.01 {
			rake = 0.01
		}
	} else {
		rake = math.Floor(rake)
		if rake <= 0 {
			rake = 1.0
		}
	}

	if hs.RakeCap != 0 {
		if rake > hs.RakeCap {
			rake = hs.RakeCap
		}
	}

	rakePaid := make(map[uint32]float64)
	for seatNo, player := range hs.PlayersInSeats {
		if !player.Inhand {
			continue
		}
		rakePaid[uint32(seatNo)] = 0
	}

	// rake from each player
	rakeFromPlayer := rake / float64(len(winners))
	if hr.chipUnit == ChipUnit_CENT {
		rakeFromPlayer = util.FloorDecimal(rakeFromPlayer, 2)
		if rakeFromPlayer < 0.01 {
			rakeFromPlayer = 0.01
		}
	} else {
		rakeFromPlayer = float64(int(rakeFromPlayer))
		if rakeFromPlayer == 0.0 {
			rakeFromPlayer = 1.0
		}
	}

	// rake from player who won money
	//rakeFromPlayer1 := float64(0.0)
	if rake > 0 {
		rakeSubtracted := make(map[uint32]float64)

		totalRakeCollected := float64(0)
		for _, winnerSeat := range winners {
			if playerReceived[winnerSeat] > rakeFromPlayer {
				rakePaid[winnerSeat] += rakeFromPlayer
				rakeSubtracted[winnerSeat] += rakeFromPlayer
				totalRakeCollected += rakeFromPlayer
			}
			if util.GreaterOrNearlyEqual(totalRakeCollected, rake) {
				break
			}
		}
		for _, pot := range potWinners {
			for _, board := range pot.BoardWinners {
				for _, handWinner := range board.HiWinners {
					seatNo := handWinner.SeatNo
					if util.GreaterOrNearlyEqual(rakeSubtracted[seatNo], rakeFromPlayer) {
						handWinner.Amount = util.RoundDecimal(handWinner.Amount-rakeFromPlayer, 2)
						rakeSubtracted[seatNo] -= rakeFromPlayer
					}
				}
				for _, handWinner := range board.LowWinners {
					seatNo := handWinner.SeatNo
					if util.GreaterOrNearlyEqual(rakeSubtracted[seatNo], rakeFromPlayer) {
						handWinner.Amount = util.RoundDecimal(handWinner.Amount-rakeFromPlayer, 2)
						rakeSubtracted[seatNo] -= rakeFromPlayer
					}
				}
			}
		}

		for seatNo, rakeAmount := range rakePaid {
			player := hs.PlayersInSeats[seatNo]
			if !player.Inhand {
				continue
			}
			if rakeAmount > 0 {
				playerStack[player.PlayerId] = util.RoundDecimal(playerStack[player.PlayerId]-rakeAmount, 2)
				playerReceived[seatNo] = util.RoundDecimal(playerReceived[seatNo]-rakeAmount, 2)
				rakePlayers[player.PlayerId] = util.RoundDecimal(rakeAmount, 2)
			}
		}
		hs.RakePaid = rakePlayers
		hs.RakeCollected = totalRakeCollected
	}
	return rakePlayers
}
func (hr *HandResultProcessor) calcRakeAndBalance(hs *HandState, potWinners []*PotWinnersV2) map[uint32]*PlayerHandInfo {
	playerStack := make(map[uint64]float64)
	playerReceived := make(map[uint32]float64)

	for seatNoIdx, player := range hs.PlayersInSeats {
		if !player.Inhand || player.SeatNo == 0 || player.OpenSeat {
			continue
		}
		playerStack[player.PlayerId] = player.Stack
		playerReceived[uint32(seatNoIdx)] = 0
	}
	totalPot := float64(0)
	winners := make([]uint32, 0)
	// update player balance
	for _, pot := range potWinners {
		totalPot += pot.Amount
		for _, board := range pot.BoardWinners {
			for _, handWinner := range board.HiWinners {
				seatNo := handWinner.SeatNo
				found := false
				for _, w := range winners {
					if w == seatNo {
						found = true
						break
					}
				}

				if !found {
					winners = append(winners, seatNo)
				}

				player := hs.PlayersInSeats[seatNo]
				playerStack[player.PlayerId] = playerStack[player.PlayerId] + handWinner.Amount
				playerReceived[seatNo] = playerReceived[seatNo] + handWinner.Amount
			}

			for _, handWinner := range board.LowWinners {
				seatNo := handWinner.SeatNo
				found := false
				for _, w := range winners {
					if w == seatNo {
						found = true
						break
					}
				}

				if !found {
					winners = append(winners, seatNo)
				}

				player := hs.PlayersInSeats[seatNo]
				playerStack[player.PlayerId] = playerStack[player.PlayerId] + handWinner.Amount
				playerReceived[seatNo] = playerReceived[seatNo] + handWinner.Amount
			}
		}
	}
	rakePlayers := make(map[uint64]float64)

	if hs.RakePercentage > 0 {
		rakePlayers = hr.adjustRake(hs, totalPot, winners, potWinners, playerStack, playerReceived)
	}

	players := make(map[uint32]*PlayerHandInfo)
	for seatNoIdx, player := range hs.PlayersInSeats {
		if !player.Inhand {
			continue
		}
		seatNo := uint32(seatNoIdx)
		players[seatNo] = &PlayerHandInfo{
			Id:          player.PlayerId,
			PlayedUntil: player.Round,
			Cards:       poker.ByteCardsToUint32Cards(hs.PlayersCards[seatNo]),
		}
	}

	// also populate current balance of the players in the table
	for seatNo, player := range hs.PlayersInSeats {
		if !player.Inhand || player.SeatNo == 0 || player.OpenSeat {
			continue
		}

		before := float64(0.0)
		for _, playerBalance := range hs.BalanceBeforeHand {
			if playerBalance.SeatNo == uint32(seatNo) {
				before = playerBalance.Balance
				break
			}
		}
		playerBalance := &HandPlayerBalance{
			Before: before,
		}
		if currentBal, ok := playerStack[player.PlayerId]; ok {
			// winner
			playerBalance = &HandPlayerBalance{
				Before: before,
				After:  currentBal,
			}
		} else {
			// other players
			playerBalance = &HandPlayerBalance{
				Before: before,
				After:  player.Stack,
			}
		}
		rakePaidAmount := float64(0.0)
		if rake, ok := rakePlayers[player.PlayerId]; ok {
			rakePaidAmount = rake
		}

		players[player.SeatNo] = &PlayerHandInfo{
			Id:          player.PlayerId,
			PlayedUntil: player.Round,
			Cards:       poker.ByteCardsToUint32Cards(hs.PlayersCards[player.SeatNo]),
			Balance:     playerBalance,
			Received:    util.RoundDecimal(playerBalance.After-playerBalance.Before, 2),
			RakePaid:    rakePaidAmount,
		}
	}
	return players
}
