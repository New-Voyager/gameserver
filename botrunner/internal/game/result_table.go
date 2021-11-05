package game

type GameResultTableRow struct {
	PlayerID       uint64 `yaml:"playerId"`
	PlayerUUID     string `yaml:"playerUuid"`
	PlayerName     string `yaml:"playerName"`
	SessionTime    uint32 `yaml:"sessionTime"`
	SessionTimeStr string `yaml:"sessionTimeStr"`
	HandsPlayed    uint32 `yaml:"handsPlayed"`
	BuyIn          int64  `yaml:"buyIn"`
	Profit         int64  `yaml:"profit"`
	Stack          int64  `yaml:"stack"`
	RakePaid       int64  `yaml:"rakePaid"`
}
