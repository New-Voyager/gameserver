package game

type GameResultTableRow struct {
	PlayerID       uint64  `yaml:"playerId"`
	PlayerUUID     string  `yaml:"playerUuid"`
	PlayerName     string  `yaml:"playerName"`
	SessionTime    uint32  `yaml:"sessionTime"`
	SessionTimeStr string  `yaml:"sessionTimeStr"`
	HandsPlayed    uint32  `yaml:"handsPlayed"`
	BuyIn          float32 `yaml:"buyIn"`
	Profit         float32 `yaml:"profit"`
	Stack          float32 `yaml:"stack"`
	RakePaid       float32 `yaml:"rakePaid"`
}
