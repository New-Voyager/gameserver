package player

import (
	"voyager.com/botrunner/internal/game"
)

// The state of the game from the player's point of view.
type gameView struct {
	status      game.GameStatus
	tableStatus game.TableStatus
	table       *tableView
	handNum     uint32
	handStatus  game.HandStatus
	handResult2 *game.HandResultClient
}

// The state of the game table from the player's point of view.
type tableView struct {
	nextActionSeat uint32
	buttonPos      uint32
	sbPos          uint32
	bbPos          uint32

	playersBySeat map[uint32]*player

	flopCards  []uint32
	turnCards  []uint32
	riverCards []uint32

	// The moves from myself and other players that have been played so far at each hand stage.
	actionTracker *game.HandActionTracker

	playersActed map[uint32]*game.PlayerActRound
}

type player struct {
	playerID uint64
	seatNo   uint32
	status   game.PlayerStatus
	stack    int64
	buyIn    int64
	isBot    bool
}
