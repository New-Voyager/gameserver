package test

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"voyager.com/server/game"
)

var testPlayerLogger = log.With().Str("logger_name", "test::testplayer").Logger()

// TestPlayer is a receiver for game and hand messages
// it also sends messages to game and hand via player object
type TestPlayer struct {
	playerInfo GamePlayer
	player *game.Player

	// we preserve the last message 
	lastMessage *game.HandMessage
}

func NewTestPlayer(playerInfo GamePlayer) *TestPlayer {
	return &TestPlayer{
		playerInfo: playerInfo,
	}
}

func (t *TestPlayer) setPlayer(player *game.Player) {
	t.player = player
}

func (t *TestPlayer) HandMessageFromGame(jsonb []byte) {
	testPlayerLogger.Info().
		Uint32("club", t.player.ClubID).
		Uint32("game", t.player.GameNum).
		Uint32("playerid", t.player.PlayerID).
		Uint32("seatNo", t.player.SeatNo).
		Str("player", t.player.PlayerName).
		Msg(fmt.Sprintf("Json: %s", string(jsonb)))
}

func (t *TestPlayer) GameMessageFromGame(jsonb []byte) {
	testPlayerLogger.Info().
		Uint32("club", t.player.ClubID).
		Uint32("game", t.player.GameNum).
		Uint32("playerid", t.player.PlayerID).
		Uint32("seatNo", t.player.SeatNo).
		Str("player", t.player.PlayerName).
		Msg(fmt.Sprintf("Json: %s", string(jsonb)))
}
