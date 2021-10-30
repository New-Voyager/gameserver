package game

import (
	"math"

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

func (hr *HandResultProcessor) adjustRake(hs *HandState, totalPot float32, potWinners []*PotWinnersV2, playerStack map[uint64]float32, playerReceived map[uint32]float32) map[uint64]float32 {
	rakePlayers := make(map[uint64]float32)
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

	rakePaid := make(map[uint32]float32)
	for seatNo, player := range hs.PlayersInSeats {
		if !player.Inhand {
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
		for totalRakeCollected < rake {
			for _, pot := range potWinners {
				if totalRakeCollected >= rake {
					break
				}
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
		}

		for seatNo, rakeAmount := range rakePaid {
			player := hs.PlayersInSeats[seatNo]
			if !player.Inhand {
				continue
			}
			if rakeAmount > 0 {
				playerStack[player.PlayerId] = playerStack[player.PlayerId] - rakeAmount
				playerReceived[seatNo] = playerReceived[seatNo] - rakeAmount
				rakePlayers[player.PlayerId] = rakeAmount
			}
		}
		hs.RakePaid = rakePlayers
		hs.RakeCollected = totalRakeCollected
	}
	return rakePlayers
}
func (hr *HandResultProcessor) calcRakeAndBalance(hs *HandState, potWinners []*PotWinnersV2) map[uint32]*PlayerHandInfo {
	playerStack := make(map[uint64]float32)
	playerReceived := make(map[uint32]float32)

	for seatNoIdx, player := range hs.PlayersInSeats {
		if !player.Inhand || player.SeatNo == 0 || player.OpenSeat {
			continue
		}
		playerStack[player.PlayerId] = player.Stack
		playerReceived[uint32(seatNoIdx)] = 0
	}
	totalPot := float32(0)
	// update player balance
	for _, pot := range potWinners {
		totalPot += pot.Amount
		for _, board := range pot.BoardWinners {
			for _, handWinner := range board.HiWinners {
				seatNo := handWinner.SeatNo
				player := hs.PlayersInSeats[seatNo]
				playerStack[player.PlayerId] = playerStack[player.PlayerId] + handWinner.Amount
				playerReceived[seatNo] = playerReceived[seatNo] + handWinner.Amount
			}

			for _, handWinner := range board.LowWinners {
				seatNo := handWinner.SeatNo
				player := hs.PlayersInSeats[seatNo]
				playerStack[player.PlayerId] = playerStack[player.PlayerId] + handWinner.Amount
				playerReceived[seatNo] = playerReceived[seatNo] + handWinner.Amount
			}
		}
	}
	rakePlayers := make(map[uint64]float32)

	if hs.RakePercentage > 0 {
		rakePlayers = hr.adjustRake(hs, totalPot, potWinners, playerStack, playerReceived)
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
		rakePaidAmount := float32(0.0)
		if rake, ok := rakePlayers[player.PlayerId]; ok {
			rakePaidAmount = rake
		}

		players[player.SeatNo] = &PlayerHandInfo{
			Id:          player.PlayerId,
			PlayedUntil: player.Round,
			Cards:       poker.ByteCardsToUint32Cards(hs.PlayersCards[player.SeatNo]),
			Balance:     playerBalance,
			Received:    playerBalance.After - playerBalance.Before,
			RakePaid:    rakePaidAmount,
		}
	}
	return players
}
