package game

import "time"

// GameInfo is the response object for gameinfo api.
type GameInfo struct {
	GameID                uint64 `json:"gameID"`
	GameCode              string
	GameType              string
	Title                 string
	SmallBlind            float64
	BigBlind              float64
	StraddleBet           float64
	UtgStraddleAllowed    bool
	ButtonStraddleAllowed bool
	MinPlayers            uint32
	MaxPlayers            uint32
	GameLength            uint32
	BuyInApproval         bool
	BreakLength           uint32
	AutoKickAfterBreak    bool
	WaitlistAllowed       bool
	SitInApproval         bool
	MaxWaitList           uint32
	RakePercentage        float64
	RakeCap               float64
	BuyInMin              float64
	BuyInMax              float64
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
	GameToken               string
	PlayerGameStatus        string
	GameToPlayerChannel     string
	HandToAllChannel        string
	PlayerToHandChannel     string
	HandToPlayerChannel     string
	HandToPlayerTextChannel string
	ClientAliveChannel      string
	Start                   bool
	BotsToWaitlist          bool
}

// SeatInfo is the info about a player sitting in a game.
type SeatInfo struct {
	SeatNo     uint32  `json:"seatNo"`
	PlayerUUID string  `json:"playerUuid"`
	PlayerId   uint64  `json:"playerId"`
	Name       string  `json:"name"`
	BuyIn      float64 `json:"buyIn"`
	Stack      float64 `json:"stack"`
	IsBot      bool    `json:"isBot"`
	Status     string  `json:"status"`
}

// GameCreateOpt contains parameters for creating a new game.
type GameCreateOpt struct {
	Title              string
	GameType           string
	SmallBlind         float64
	BigBlind           float64
	UtgStraddleAllowed bool
	StraddleBet        float64
	MinPlayers         int
	MaxPlayers         int
	GameLength         int
	BuyInApproval      bool
	RakePercentage     float64
	RakeCap            float64
	BuyInMin           float64
	BuyInMax           float64
	ActionTime         int
	RewardIds          []uint32
	RunItTwiceAllowed  bool
	MuckLosingHand     bool
	RoeGames           []string
	DealerChoiceGames  []string
	HighHandTracked    bool
	AppCoinsNeeded     bool
	IpCheck            bool
	GpsCheck           bool
	DealerChoiceOrbit  bool
}
