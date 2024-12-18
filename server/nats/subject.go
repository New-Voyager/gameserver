package nats

import (
	"fmt"
)

func GetGame2AllPlayerSubject(gameCode string) string {
	return fmt.Sprintf("game.%s.player", gameCode)
}

func GetGame2PlayerSubject(gameCode string, playerID uint64) string {
	return fmt.Sprintf("game.%s.player.%d", gameCode, playerID)
}

func GetHand2AllPlayerSubject(gameCode string) string {
	return fmt.Sprintf("hand.%s.player.all", gameCode)
}

func GetPlayer2HandSubject(gameCode string) string {
	return fmt.Sprintf("player.%s.hand", gameCode)
}

func GetHand2PlayerSubject(gameCode string, playerID uint64) string {
	return fmt.Sprintf("hand.%s.player.%d", gameCode, playerID)
}

func GetClientAliveSubject(gameCode string) string {
	return fmt.Sprintf("clientalive.%s", gameCode)
}

func GetTournamentPlayerSubject(tournamentID uint32, playerID uint64) string {
	return fmt.Sprintf("tournament.%d.player.%d", tournamentID, playerID)
}
