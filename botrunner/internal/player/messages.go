package player

const (
	// BotDriverSetupDeck is the message type for SetupDeck message.
	BotDriverSetupDeck string = "B2GSetupDeck"
)

// SetupDeck is the message used by the bot runner to tell the game server to
// pre-setup the deck with the specific cards before dealing cards.
type SetupDeck struct {
	MessageType          string       `json:"message-type"`
	GameCode             string       `json:"game-code"`
	GameID               uint64       `json:"game-id"`
	ButtonPos            uint32       `json:"button-pos"`
	Shuffle              bool         `json:"shuffle"`
	Board                []string     `json:"board"`
	Board2               []string     `json:"board2"`
	Flop                 []string     `json:"flop"`
	Turn                 string       `json:"turn"`
	River                string       `json:"river"`
	PlayerCards          []PlayerCard `json:"player-cards"`
	Auto                 bool         `json:"auto"`
	Pause                uint32       `json:"pause"`
	BombPot              bool         `json:"bomb-pot"`
	BombPotBet           uint32       `json:"bomb-pot-bet"`
	DoubleBoard          bool         `json:"double-board"`
	IncludeStatsInResult bool         `json:"include-stats"`
	ResultPauseTime      uint32       `json:"result-pause-time"`
}

type PlayerCard struct {
	Seat  uint32   `json:"seat"`
	Cards []string `json:"cards"`
}
