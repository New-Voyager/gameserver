package player

const (
	// BotDriverSetupDeck is the message type for SetupDeck message.
	BotDriverSetupDeck string = "B2GSetupDeck"
)

// SetupDeck is the message used by the bot runner to tell the game server to
// pre-setup the deck with the specific cards before dealing cards.
type SetupDeck struct {
	MessageType string       `json:"message-type"`
	GameCode    string       `json:"game-code"`
	GameID      uint64       `json:"game-id"`
	ButtonPos   uint32       `json:"button-pos"`
	Shuffle     bool         `json:"shuffle"`
	Flop        []string     `json:"flop"`
	Turn        string       `json:"turn"`
	River       string       `json:"river"`
	PlayerCards []PlayerCard `json:"player-cards"`
	Auto        bool         `json:"auto"`
	Pause       uint32       `json:"pause"`
}

type PlayerCard struct {
	Cards []string `json:"cards"`
}
