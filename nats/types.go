package nats

type GameConfig struct {
	ClubId             int     `json:"clubId"`
	GameId             int     `json:"gameId"`
	GameType           int     `json:"gameType"`
	ClubCode           string  `json:"clubCode"`
	GameCode           string  `json:"gameCode"`
	Title              string  `json:"title"`
	SmallBlind         float64 `json:"smallBlind"`
	BigBlind           float64 `json:"bigBlind"`
	StraddleBet        float64 `json:"straddleBet"`
	MinPlayers         float64 `json:"minPlayers"`
	MaxPlayers         float64 `json:"maxPlayers"`
	GameLength         int     `json:"gameLength"`
	RakePercentage     float64 `json:"rakePercentage"`
	RakeCap            float64 `json:"rakeCap"`
	BuyInMin           float64 `json:"buyInMin"`
	BuyInMax           float64 `json:"buyInMax"`
	ActionTime         int     `json:"actionTime"`
	StartedBy          string  `json:"startedBy"`
	StartedByUuid      string  `json:"startedByUuid"`
	BreakLength        int     `json:"breakLength"`
	AutoKickAfterBreak bool    `json:"autoKickAfterBreak"`
}
