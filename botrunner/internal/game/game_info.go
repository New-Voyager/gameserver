package game

import "time"

// GameInfo is the response object for gameinfo api.
type GameInfo struct {
	GameCode              string
	GameType              string
	Title                 string
	SmallBlind            float32
	BigBlind              float32
	StraddleBet           float32
	UtgStraddleAllowed    bool
	ButtonStraddleAllowed bool
	MinPlayers            uint32
	MaxPlayers            uint32
	GameLength            uint32
	BuyInApproval         bool
	BreakLength           uint32
	AutoKickAfterBreak    bool
	WaitlistSupported     bool
	SitInApproval         bool
	MaxWaitList           uint32
	RakePercentage        float32
	RakeCap               float32
	BuyInMin              float32
	BuyInMax              float32
	ActionTime            uint32
	MuckLosingHand        bool
	RunItTwiceAllowed     bool
	WaitForBigBlind       bool
	StartedBy             string
	StartedAt             time.Time
	EndedBy               string
	EndedAt               time.Time
	Template              bool
	Status                string
	TableStatus           string
	SeatInfo              struct {
		AvailableSeats []uint32   `json:"availableSeats"`
		PlayersInSeats []SeatInfo `json:"playersInSeats"`
	} `json:"seatInfo"`
	GameToken           string
	PlayerGameStatus    string
	GameToPlayerChannel string
	HandToAllChannel    string
	PlayerToHandChannel string
	HandToPlayerChannel string
	PingToPlayerChannel string
	PongChannel         string
	Start               bool
}

// SeatInfo is the info about a player sitting in a game.
type SeatInfo struct {
	SeatNo     uint32  `json:"seatNo"`
	PlayerUUID string  `json:"playerUuid"`
	PlayerId   uint64  `json:"playerId"`
	Name       string  `json:"name"`
	BuyIn      float32 `json:"buyIn"`
	Stack      float32 `json:"stack"`
	IsBot      bool    `json:"isBot"`
	Status     string  `json:"status"`
}

// GameCreateOpt contains parameters for creating a new game.
type GameCreateOpt struct {
	Title              string
	GameType           string
	SmallBlind         float32
	BigBlind           float32
	UtgStraddleAllowed bool
	StraddleBet        float32
	MinPlayers         int
	MaxPlayers         int
	GameLength         int
	BuyInApproval      bool
	RakePercentage     float32
	RakeCap            float32
	BuyInMin           float32
	BuyInMax           float32
	ActionTime         int
	RewardIds          []uint32
	RunItTwiceAllowed  bool
	MuckLosingHand     bool
	RoeGames           []string
	DealerChoiceGames  []string
}
