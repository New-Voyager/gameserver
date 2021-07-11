package nats

import (
	"fmt"
)

func GetGame2AllPlayerSubject(gameCode string) string {
	return fmt.Sprintf("game.%s.player", gameCode)
}

func GetHand2AllPlayerSubject(gameCode string) string {
	return fmt.Sprintf("hand.%s.player.all", gameCode)
}

func GetPlayer2HandSubject(gameCode string) string {
	return fmt.Sprintf("player.%s.hand", gameCode)
}

func GetPingSubject(gameCode string) string {
	return fmt.Sprintf("ping.%s", gameCode)
}

func GetPongSubject(gameCode string) string {
	return fmt.Sprintf("pong.%s", gameCode)
}
