package game

import (
	"fmt"
	"io/ioutil"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// Script contains the YAML contents.
type Script struct {
	SetupScript string `yaml:"setup-script"`
	Hands       []Hand `yaml:"hands"`
	Game        Game   `yaml:"game"`
	ResetDB     bool   `yaml:"reset-db"`
	GameTitle   string `yaml:"game-title"`
}

// BotRunnerConfig is equivalent to Script except that the SetupScript file is parsed into SetupConfig.
type BotRunnerConfig struct {
	Setup     SetupConfig
	Hands     []Hand
	Game      Game
	GameTitle string
	ResetDB   bool
}

// SetupConfig holds the contents of the setup yaml.
type SetupConfig struct {
	Players []PlayerConfig
	Club    ClubConfig
	Game    GameConfig
	SitIn   []SeatConfig  `yaml:"sitIn"`
	BuyIn   []BuyInConfig `yaml:"buyIn"`
}

// Hand script
type Hand struct {
	Num           uint32         `yaml:"num"`
	Setup         HandSetup      `yaml:"setup"`
	PreflopAction BettingRound   `yaml:"preflop-action"`
	FlopAction    BettingRound   `yaml:"flop-action"`
	TurnAction    BettingRound   `yaml:"turn-action"`
	RiverAction   BettingRound   `yaml:"river-action"`
	Result        TestHandResult `yaml:"result"`
}

// game setting
type Game struct {
	AutoPlay bool `yaml:"auto-play"`
}

type PlayerConfig struct {
	Name               string
	DeviceID           string `yaml:"deviceId"`
	Email              string
	Password           string
	Bot                bool
	BotActionPauseTime uint32 `yaml:"botActionPauseTime"`
}

type Reward struct {
	Id       int
	Name     string
	Type     string
	Amount   float32
	Schedule string
}

type ClubConfig struct {
	Name        string
	Description string
	Rewards     []Reward
}

type GameConfig struct {
	Create             bool
	Title              string  `yaml:"title"`
	GameType           string  `yaml:"gameType"`
	SmallBlind         float32 `yaml:"smallBlind"`
	BigBlind           float32 `yaml:"bigBlind"`
	UtgStraddleAllowed bool    `yaml:"utgStraddleAllowed"`
	StraddleBet        float32 `yaml:"straddleBet"`
	MinPlayers         int     `yaml:"minPlayers"`
	MaxPlayers         int     `yaml:"maxPlayers"`
	GameLength         int     `yaml:"gameLength"`
	BuyInApproval      bool    `yaml:"buyInApproval"`
	RakePercentage     float32 `yaml:"rakePercentage"`
	RakeCap            float32 `yaml:"rakeCap"`
	BuyInMin           float32 `yaml:"buyInMin"`
	BuyInMax           float32 `yaml:"buyInMax"`
	ActionTime         int     `yaml:"actionTime"`
	Rewards            string
}

type SeatConfig struct {
	SeatNo     uint32 `yaml:"seatNo"`
	PlayerName string `yaml:"playerName"`
}

type BuyInConfig struct {
	SeatNo   uint32  `yaml:"seatNo"`
	BuyChips float32 `yaml:"buyChips"`
}

type SeatChange struct {
	SeatNo  uint32 `yaml:"seatNo"`
	Confirm bool   `yaml:"confirm"`
}

type LeaveGame struct {
	SeatNo uint32 `yaml:"seatNo"`
}

type WaitList struct {
	Player  string  `yaml:"player"`
	Confirm bool    `yaml:"confirm"`
	BuyIn   float32 `yaml:"buyIn"`
}
type HandSetup struct {
	ButtonPos  uint32               `yaml:"button-pos"`
	Flop       []string             `yaml:"flop"`
	Turn       string               `yaml:"turn"`
	River      string               `yaml:"river"`
	SeatCards  []TestSeatCards      `yaml:"seat-cards"`
	Verify     HandSetupVerfication `yaml:"verify"`
	Auto       bool                 `yaml:"auto"`
	SeatChange []SeatChange         `yaml:"seat-change"` // players requesting seat-change
	LeaveGame  []LeaveGame          `yaml:"leave-game"`
	WaitLists  []WaitList           `yaml:"wait-list"`
	Pause      uint32               `yaml:"pause"` // bot runner pauses and waits before next hand
}

type BettingRound struct {
	Actions     []TestHandAction   `yaml:"actions"`
	SeatActions []string           `yaml:"seat-actions"`
	Verify      VerifyBettingRound `yaml:"verify"`
}

type HighHandSeat struct {
	SeatNo uint32 `yaml:"seat-no"`
}

type TestHandResult struct {
	Winners       []TestHandWinner `yaml:"winners"`
	ActionEndedAt string           `yaml:"action-ended"`
	Stacks        []PlayerStack    `yaml:"stacks"`
	HighHand      []HighHandSeat   `yaml:"high-hand"`
}

type TestSeatCards struct {
	Cards  []string `yaml:"cards"`
	SeatNo uint32   `yaml:"seat-no"`
}

type HandSetupVerfication struct {
	Button        uint32          `yaml:"button"`
	SB            uint32          `yaml:"sb"`
	BB            uint32          `yaml:"bb"`
	NextActionPos uint32          `yaml:"next-action-pos"`
	State         string          `yaml:"state"`
	DealtCards    []TestSeatCards `yaml:"dealt-cards"`
}

type TestHandAction struct {
	SeatNo uint32  `yaml:"seat"`
	Action string  `yaml:"action"`
	Amount float32 `yaml:"amount"`
}

type VerifyBettingRound struct {
	State        string   `yaml:"state"`
	Board        []string `yaml:"board"`
	Pots         []Pot    `yaml:"pots"`
	NoMoreAction bool     `yaml:"no-more-action"`
}

type TestHandWinner struct {
	Seat    uint32  `yaml:"seat"`
	Receive float32 `yaml:"receive"`
	RankStr string  `yaml:"rank"`
}

type PlayerStack struct {
	Seat  uint32  `yaml:"seat"`
	Stack float32 `yaml:"stack"`
}

type Pot struct {
	Pot        float32  `yaml:"pot"`
	SeatsInPot []uint32 `yaml:"seats"`
}

// ParseYAMLConfig parses the botrunner yaml script into a Config object.
func ParseYAMLConfig(fileName string) (*BotRunnerConfig, error) {
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Error reading config file [%s]", fileName))
	}

	var data Script
	err = yaml.Unmarshal(bytes, &data)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Error parsing YAML file [%s]", fileName))
	}

	setupFileName := data.SetupScript
	setupFileBytes, err := ioutil.ReadFile(setupFileName)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Error reading setup config file [%s]", setupFileName))
	}

	var setupConfig SetupConfig
	err = yaml.Unmarshal(setupFileBytes, &setupConfig)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("Error parsing YAML file [%s]", setupFileName))
	}

	var config *BotRunnerConfig = &BotRunnerConfig{
		Setup:     setupConfig,
		Hands:     data.Hands,
		Game:      data.Game,
		ResetDB:   data.ResetDB,
		GameTitle: data.GameTitle,
	}

	return config, nil
}

// GetBuyInAmount returns the buy-in amount for the specified seat.
func (c *BotRunnerConfig) GetBuyInAmount(seatNo uint32) float32 {
	for _, buyIn := range c.Setup.BuyIn {
		if buyIn.SeatNo == seatNo {
			return buyIn.BuyChips
		}
	}
	return 0
}

// GetSeatNoByPlayerName returns the seat number for the specified player name.
func (c *BotRunnerConfig) GetSeatNoByPlayerName(playerName string) uint32 {
	for _, sitIn := range c.Setup.SitIn {
		if sitIn.PlayerName == playerName {
			return sitIn.SeatNo
		}
	}
	return 0
}

// GetPlayerNameBySeatNo returns the player name for the specified seat number.
func (c *BotRunnerConfig) GetPlayerNameBySeatNo(seatNo uint32) string {
	for _, sitIn := range c.Setup.SitIn {
		if sitIn.SeatNo == seatNo {
			return sitIn.PlayerName
		}
	}
	return "MISSING"
}

// IsSeatHuman returns true if the player designated for the specified seat is a human player and not a bot.
func (c *BotRunnerConfig) IsSeatHuman(seatNo uint32) bool {
	playerName := c.GetPlayerNameBySeatNo(seatNo)
	for _, p := range c.Setup.Players {
		if p.Name == playerName {
			return !p.Bot
		}
	}
	return false
}

// IsAutoPlay returns true if the bots are supposed to make their own decision
// as opposed to following the scripted hand actions.
func (c *BotRunnerConfig) IsAutoPlay() bool {
	return c.Game.AutoPlay
}
