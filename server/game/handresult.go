package game

import (
	"fmt"

	"voyager.com/server/poker"
)

type HandResultProcessor struct {
	handState         *HandState
	rewardTrackingIds []uint32
	evaluator         HandEvaluator
}

func NewHandResultProcessor(handState *HandState, maxSeats uint32, rewardTrackingIds []uint32) *HandResultProcessor {
	var evaluator HandEvaluator
	includeHighHand := rewardTrackingIds != nil
	if handState.GameType == GameType_HOLDEM {
		evaluator = NewHoldemWinnerEvaluate(handState, includeHighHand, maxSeats)
	} else if handState.GameType == GameType_PLO || handState.GameType == GameType_FIVE_CARD_PLO {
		evaluator = NewPloWinnerEvaluate(handState, includeHighHand, false, maxSeats)
	} else if handState.GameType == GameType_PLO_HILO || handState.GameType == GameType_FIVE_CARD_PLO_HILO {
		evaluator = NewPloWinnerEvaluate(handState, includeHighHand, true, maxSeats)
	}
	return &HandResultProcessor{
		handState:         handState,
		rewardTrackingIds: rewardTrackingIds,
		evaluator:         evaluator,
	}
}

func (hr *HandResultProcessor) getWinners() map[uint32]*PotWinners {
	return hr.evaluator.GetWinners()
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
		hr.handState.setWinners(hr.getWinners())
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
		fmt.Printf("\n\n================================================================\n\n")
		for seatNo, hand := range bestSeatHands {
			if highHand, ok := highHands[seatNo]; ok {
				fmt.Printf("Seat: %d, Cards:%+v, Str: %s Rank: %d, rankStr: %s, hhHand: %s rank: %d rankStr: %s\n",
					seatNo,
					hand.cards,
					poker.CardsToString(hand.cards), hand.rank, poker.RankString(hand.rank),
					poker.CardToString(highHand.cards), highHand.rank, poker.RankString((highHand.rank)))
			} else {
				fmt.Printf("Seat: %d, Cards:%+v, Str: %s Rank: %d, rankStr: %s\n",
					seatNo,
					hand.cards,
					poker.CardsToString(hand.cards), hand.rank, poker.RankString(hand.rank))
			}
		}
		fmt.Printf("\n\n================================================================\n\n")
	}
	handResult := &HandResult{
		GameId:   hr.handState.GameId,
		HandNum:  hr.handState.HandNum,
		GameType: hr.handState.GameType,
	}

	// update stats in the result
	handResult.PlayerStats = hr.handState.PlayerStats
	handResult.HandStats = hr.handState.HandStats

	// get hand log
	handResult.HandLog = hr.handState.getLog()

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
