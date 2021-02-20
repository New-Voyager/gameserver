package scriptreader

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Script contains game script YAML content.
type Script struct {
	Club          string         `yaml:"club"`
	Game          Game           `yaml:"game"`
	StartingSeats []StartingSeat `yaml:"starting-seats"`
	BotConfig     BotConfig      `yaml:"bot-config"`
	Hands         []Hand         `yaml:"hands"`
}

// Game contains game configuration in the game script.
type Game struct {
	Create             bool    `yaml:"create"`
	Title              string  `yaml:"title"`
	GameType           string  `yaml:"game-type"`
	SmallBlind         float32 `yaml:"small-blind"`
	BigBlind           float32 `yaml:"big-blind"`
	UtgStraddleAllowed bool    `yaml:"utg-straddle-allowed"`
	StraddleBet        float32 `yaml:"straddle-bet"`
	MinPlayers         int     `yaml:"min-players"`
	MaxPlayers         int     `yaml:"max-players"`
	GameLength         int     `yaml:"game-length"`
	BuyInApproval      bool    `yaml:"buy-in-approval"`
	RakePercentage     float32 `yaml:"rake-percentage"`
	RakeCap            float32 `yaml:"rake-cap"`
	BuyInMin           float32 `yaml:"buy-in-min"`
	BuyInMax           float32 `yaml:"buy-in-max"`
	ActionTime         int     `yaml:"action-time"`
	Rewards            string  `yaml:"rewards"`
}

// StartingSeat contains an entry in the StartingSeats array in the game script.
type StartingSeat struct {
	SeatNo uint32  `yaml:"seat-no"`
	Player string  `yaml:"player"`
	BuyIn  float32 `yaml:"buy-in"`
}

// BotConfig contains botConfig content in the game script.
type BotConfig struct {
	MinActionPauseTime uint32 `yaml:"min-action-pause-time"`
	MaxActionPauseTime uint32 `yaml:"max-action-pause-time"`
}

// Hand contains an entry in the hands array in the game script.
type Hand struct {
	Num     uint32       `yaml:"num"`
	Setup   HandSetup    `yaml:"setup"`
	Preflop BettingRound `yaml:"preflop"`
	Flop    BettingRound `yaml:"flop"`
	Turn    BettingRound `yaml:"turn"`
	River   BettingRound `yaml:"river"`
	Result  HandResult   `yaml:"result"`
}

// HandSetup contains the setup content in the hand config.
type HandSetup struct {
	ButtonPos  uint32               `yaml:"button-pos"`
	Flop       []string             `yaml:"flop"`
	Turn       string               `yaml:"turn"`
	River      string               `yaml:"river"`
	SeatCards  []SeatCards          `yaml:"seat-cards"`
	Verify     HandSetupVerfication `yaml:"verify"`
	Auto       bool                 `yaml:"auto"`
	SeatChange []SeatChange         `yaml:"seat-change"` // players requesting seat-change
	LeaveGame  []LeaveGame          `yaml:"leave-game"`
	WaitLists  []WaitList           `yaml:"wait-list"`
	Pause      uint32               `yaml:"pause"` // bot runner pauses and waits before next hand
}

type SeatCards struct {
	Cards  []string `yaml:"cards"`
	SeatNo uint32   `yaml:"seat-no"`
}

type HandSetupVerfication struct {
	Button        uint32      `yaml:"button"`
	SB            uint32      `yaml:"sb"`
	BB            uint32      `yaml:"bb"`
	NextActionPos uint32      `yaml:"next-action-pos"`
	State         string      `yaml:"state"`
	DealtCards    []SeatCards `yaml:"dealt-cards"`
}

type SeatChange struct {
	SeatNo  uint32 `yaml:"seat-no"`
	Confirm bool   `yaml:"confirm"`
}

type LeaveGame struct {
	SeatNo uint32 `yaml:"seat-no"`
}

type WaitList struct {
	Player  string  `yaml:"player"`
	Confirm bool    `yaml:"confirm"`
	BuyIn   float32 `yaml:"buy-in"`
}

type BettingRound struct {
	SeatActions []SeatAction             `yaml:"seat-actions"`
	Verify      BettingRoundVerification `yaml:"verify"`
}

type SeatAction struct {
	Action    Action    `yaml:"action"`
	PreAction PreAction `yaml:"pre-action"`
}

type Action struct {
	SeatNo uint32  `yaml:"seat"`
	Action string  `yaml:"action"`
	Amount float32 `yaml:"amount"`
}

// Custom unmarshaller for action expression.
// 1, FOLD
// 1, CALL, 2
func (a *Action) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var v interface{}
	var err error
	err = unmarshal(&v)
	if err != nil {
		return err
	}
	actionExpr, ok := v.(string)
	if !ok {
		return fmt.Errorf("Cannot parse action expression [%v] as string", v)
	}
	tokens := strings.Split(actionExpr, ",")
	if len(tokens) != 2 && len(tokens) != 3 {
		return fmt.Errorf("Invalid action expression string [%v]. Need 2 or 3 comma-separated tokens", v)
	}

	// Parse seat number token
	trimmed := strings.Trim(tokens[0], " ")
	seatNo, err := strconv.ParseUint(trimmed, 10, 32)
	if err != nil {
		return errors.Wrapf(err, "Cannot convert first token [%s] to seat number", trimmed)
	}

	// Parse amount token
	var amount float64
	if len(tokens) == 3 {
		trimmed := strings.Trim(tokens[2], " ")
		amount, err = strconv.ParseFloat(trimmed, 32)
		if err != nil {
			return errors.Wrapf(err, "Cannot convert third token [%s] to seat number", trimmed)
		}
	}
	a.SeatNo = uint32(seatNo)
	a.Action = strings.Trim(tokens[1], " ")
	a.Amount = float32(amount)
	return nil
}

type PreAction struct {
	SetupServerCrash SetupServerCrash       `yaml:"setup-server-crash"`
	Verify           YourActionVerification `yaml:"verify"`
}

type SetupServerCrash struct {
	CrashPoint string `yaml:"crash-point"`
}

type YourActionVerification struct {
	AvailableActions []string    `yaml:"available-actions"`
	StraddleAmount   float32     `yaml:"straddle-amount"`
	CallAmount       float32     `yaml:"call-amount"`
	RaiseAmount      float32     `yaml:"raise-amount"`
	MinBetAmount     float32     `yaml:"min-bet-amount"`
	MaxBetAmount     float32     `yaml:"max-bet-amount"`
	MinRaiseAmount   float32     `yaml:"min-raise-amount"`
	MaxRaiseAmount   float32     `yaml:"max-raise-amount"`
	AllInAmount      float32     `yaml:"all-in-amount"`
	BetOptions       []BetOption `yaml:"bet-options"`
}

type BetOption struct {
	Text   string
	Amount float32
}

// Custom unmarshaller for BetOption expression.
// All-In, 500
// Pot, 200
func (b *BetOption) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var v interface{}
	var err error
	err = unmarshal(&v)
	if err != nil {
		return err
	}
	expr, ok := v.(string)
	if !ok {
		return fmt.Errorf("Cannot parse BetOption expression [%v] as string", v)
	}
	tokens := strings.Split(expr, ",")
	if len(tokens) != 2 {
		return fmt.Errorf("Invalid BetOption expression string [%v]. Need 2 comma-separated tokens", v)
	}

	// Parse amount token
	var amount float64
	trimmed := strings.Trim(tokens[1], " ")
	amount, err = strconv.ParseFloat(trimmed, 32)
	if err != nil {
		return errors.Wrapf(err, "Cannot convert second token [%s] to BetOption amount", trimmed)
	}
	b.Text = strings.Trim(tokens[0], " ")
	b.Amount = float32(amount)
	return nil
}

type BettingRoundVerification struct {
	State        string   `yaml:"state"`
	Board        []string `yaml:"board"`
	Pots         []Pot    `yaml:"pots"`
	NoMoreAction bool     `yaml:"no-more-action"`
}

type Pot struct {
	Pot        float32  `yaml:"pot"`
	SeatsInPot []uint32 `yaml:"seats"`
}

type HandResult struct {
	Winners       []HandWinner   `yaml:"winners"`
	ActionEndedAt string         `yaml:"action-ended"`
	Stacks        []PlayerStack  `yaml:"stacks"`
	HighHand      []HighHandSeat `yaml:"high-hand"`
}

type HandWinner struct {
	Seat    uint32  `yaml:"seat"`
	Receive float32 `yaml:"receive"`
	RankStr string  `yaml:"rank"`
}

type PlayerStack struct {
	Seat  uint32  `yaml:"seat"`
	Stack float32 `yaml:"stack"`
}

type HighHandSeat struct {
	Seat uint32 `yaml:"seat"`
}

// ReadGameScript reads game script yaml file.
func ReadGameScript(fileName string) (*Script, error) {
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, errors.Wrapf(err, "Error reading game script file [%s]", fileName)
	}

	var data Script
	err = yaml.Unmarshal(bytes, &data)
	if err != nil {
		return nil, errors.Wrapf(err, "Error parsing YAML file [%s]", fileName)
	}

	return &data, nil
}
