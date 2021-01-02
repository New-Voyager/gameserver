package game

import (
	"fmt"

	"voyager.com/server/poker"
)

type HandResultProcessor struct {
	handState         *HandState
	gameState         *GameState
	rewardTrackingIds []uint32
	evaluator         HandEvaluator
}

func NewHandResultProcessor(handState *HandState, gameState *GameState, rewardTrackingIds []uint32) *HandResultProcessor {
	var evaluator HandEvaluator
	includeHighHand := gameState.RewardTrackingIds != nil
	if handState.GameType == GameType_HOLDEM {
		evaluator = NewHoldemWinnerEvaluate(gameState, handState, includeHighHand)
	} else if handState.GameType == GameType_PLO || handState.GameType == GameType_FIVE_CARD_PLO {
		evaluator = NewPloWinnerEvaluate(gameState, handState, includeHighHand, false)
	} else if handState.GameType == GameType_PLO_HILO || handState.GameType == GameType_FIVE_CARD_PLO_HILO {
		evaluator = NewPloWinnerEvaluate(gameState, handState, includeHighHand, true)
	}
	return &HandResultProcessor{
		handState:         handState,
		gameState:         gameState,
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
	handResult.Turn = hr.handState.TurnCard
	handResult.River = hr.handState.RiverCard
	if hr.handState.BoardCards != nil {
		handResult.BoardCards = make([]uint32, len(hr.handState.BoardCards))
		for i, card := range hr.handState.BoardCards {
			handResult.BoardCards[i] = uint32(card)
		}
	}

	if hr.handState.BoardCards_2 != nil {
		handResult.BoardCards_2 = make([]uint32, len(hr.handState.BoardCards_2))
		for i, card := range hr.handState.BoardCards {
			handResult.BoardCards_2[i] = uint32(card)
		}
	}

	if hr.handState.FlopCards != nil {
		handResult.Flop = make([]uint32, len(hr.handState.FlopCards))
		for i, card := range hr.handState.FlopCards {
			handResult.Flop[i] = uint32(card)
		}
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

	// we want to evaulate the hands again for the high hand if the remaining player may have the high hand
	if (hr.handState.BoardCards != nil && hr.gameState.RewardTrackingIds != nil) ||
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
		GameId:   hr.gameState.GameId,
		HandNum:  hr.handState.HandNum,
		GameType: hr.gameState.GameType,
	}

	// get hand log
	handResult.HandLog = hr.handState.getLog()

	handResult.RewardTrackingIds = hr.rewardTrackingIds
	hr.populateCommunityCards(handResult)

	handResult.Players = make(map[uint32]*PlayerInfo, 0)

	// populate players section
	for seatNoIdx, playerID := range hr.handState.GetPlayersInSeats() {

		// no player in the seat
		if playerID == 0 {
			continue
		}
		playerState, _ := hr.handState.GetPlayersState()[playerID]

		// determine whether the player has folded
		playerFolded := false
		if hr.handState.ActiveSeats[seatNoIdx] == 0 {
			playerFolded = true
		} else {
			playerState.Round = hr.handState.HandCompletedAt
		}

		seatNo := uint32(seatNoIdx + 1)

		// calculate high rank only the player hasn't folded
		rank := uint32(0xFFFFFFFF)
		highHandRank := uint32(0xFFFFFFFF)
		var bestCards []uint32
		var highHandBestCards []uint32

		cards := hr.handState.PlayersCards[seatNo]
		playerCards := make([]uint32, len(cards))
		for i, card := range cards {
			playerCards[i] = uint32(card)
		}
		if !playerFolded {
			var evaluatedCards *EvaluatedCards
			if bestSeatHands != nil {
				evaluatedCards = bestSeatHands[seatNo]
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
				if highHands[seatNo] != nil {
					highHandRank = uint32(highHands[seatNo].rank)
					highHandBestCards = highHands[seatNo].GetCards()
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
		handResult.Players[seatNo] = playerInfo
	}

	return handResult
}
