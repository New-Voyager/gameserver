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
	Board       []string     `json:"board"`
	Board2      []string     `json:"board2"`
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

type NonProtoMessage struct {
	Type       string `json:"type"`
	GameCode   string `json:"gameCode"`
	OpenedSeat uint32 `json:"openedSeat"`
	PlayerName string `json:"playerName"`
	PlayerID   uint64 `json:"playerId"`
	PlayerUUID string `json:"playerUuid"`
	ExpTime    string `json:"expTime"`
	PromptSecs int    `json:"promptSecs"`
	OldSeatNo  int    `json:"oldSeatNo"`
	NewSeatNo  int    `json:"newSeatNo"`
	RequestID  string `json:"requestId"`
}
