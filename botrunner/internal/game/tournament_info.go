package game

// TournamentInfo is the response object for TournamentInfo api.
type TournamentInfo struct {
	ID                uint64  `json:"id"`
	TournamentChannel string  `json:"tournamentChannel"`
	PrivateChannel    string  `json:"privateChannel"`
	MinPlayers        int32   `json:"minPlayers"`
	MaxPlayers        int32   `json:"maxPlayers"`
	StartingChips     float32 `json:"startingChips"`
	MaxPlayersInTable int32   `json:"maxPlayersInTable"`
}

type TournamentTableInfo struct {
	GameID                  uint64     `json:"gameID"`
	GameCode                string     `json:"gameCode"`
	TournamentChannel       string     `json:"tournamentChannel"`
	MinPlayers              int32      `json:"minPlayers"`
	MaxPlayers              int32      `json:"maxPlayers"`
	StartingChips           float32    `json:"startingChips"`
	MaxPlayersInTable       int32      `json:"maxPlayersInTable"`
	Players                 []SeatInfo `json:"players"`
	PlayerGameStatus        string
	GameToPlayerChannel     string
	HandToAllChannel        string
	PlayerToHandChannel     string
	HandToPlayerChannel     string
	HandToPlayerTextChannel string
	ClientAliveChannel      string
	Playing                 bool
	TableNo                 int32
}
