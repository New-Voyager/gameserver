package game

type GameResultTableRow struct {
	PlayerID       uint64  `yaml:"playerId"`
	PlayerUUID     string  `yaml:"playerUuid"`
	PlayerName     string  `yaml:"playerName"`
	SessionTime    uint32  `yaml:"sessionTime"`
	SessionTimeStr string  `yaml:"sessionTimeStr"`
	HandsPlayed    uint32  `yaml:"handsPlayed"`
	BuyIn          float64 `yaml:"buyIn"`
	Profit         float64 `yaml:"profit"`
	Stack          float64 `yaml:"stack"`
	RakePaid       float64 `yaml:"rakePaid"`
}
