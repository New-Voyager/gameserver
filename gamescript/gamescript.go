package gamescript

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	mapset "github.com/deckarep/golang-set"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Script contains game script YAML content.
type Script struct {
	AppGame       string         `yaml:"app-game"`
	Club          Club           `yaml:"club"`
	Game          Game           `yaml:"game"`
	StartingSeats []StartingSeat `yaml:"starting-seats"`
	Tester        string         `yaml:"tester"`
	BotConfig     BotConfig      `yaml:"bot-config"`
	AutoPlay      bool           `yaml:"auto-play"`
	Hands         []Hand         `yaml:"hands"`
	Observers     []Observer     `yaml:"observers"`
}

type Club struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Rewards     []Reward
}

type Reward struct {
	Name     string
	Type     string
	Amount   float32
	Schedule string
}

// Game contains game configuration in the game script.
type Game struct {
	Create             bool     `yaml:"create"`
	Title              string   `yaml:"title"`
	GameType           string   `yaml:"game-type"`
	SmallBlind         float32  `yaml:"small-blind"`
	BigBlind           float32  `yaml:"big-blind"`
	UtgStraddleAllowed bool     `yaml:"utg-straddle-allowed"`
	StraddleBet        float32  `yaml:"straddle-bet"`
	MinPlayers         int      `yaml:"min-players"`
	MaxPlayers         int      `yaml:"max-players"`
	GameLength         int      `yaml:"game-length"`
	BuyInApproval      bool     `yaml:"buy-in-approval"`
	RakePercentage     float32  `yaml:"rake-percentage"`
	RakeCap            float32  `yaml:"rake-cap"`
	BuyInMin           float32  `yaml:"buy-in-min"`
	BuyInMax           float32  `yaml:"buy-in-max"`
	ActionTime         int      `yaml:"action-time"`
	Rewards            string   `yaml:"rewards"`
	DontStart          bool     `yaml:"dont-start"`
	RunItTwiceAllowed  bool     `yaml:"run-it-twice-allowed"`
	MuckLosingHand     bool     `yaml:"muck-losing-hand"`
	RoeGames           []string `yaml:"roe-games"`
	DealerChoiceGames  []string `yaml:"dealer-choice-games"`
	HighHandTracked    bool     `yaml:"highhand-tracked"`
}

// StartingSeat contains an entry in the StartingSeats array in the game script.
type StartingSeat struct {
	Seat                     uint32  `yaml:"seat"`
	Player                   string  `yaml:"player"`
	BuyIn                    float32 `yaml:"buy-in"`
	RunItTwice               bool    `yaml:"run-it-twice"`
	RunItTwicePromptResponse bool    `yaml:"run-it-twice-prompt"`
	MuckLosingHand           bool    `yaml:"muck-losing-hand"`
	Reload                   *bool   `yaml:"reload"`
}

// Observer contains entries of observers of game
type Observer struct {
	Player   string  `yaml:"player"`
	Waitlist bool    `yaml:"waitlist"`
	BuyIn    float32 `yaml:"buy-in"`
	Confirm  bool    `yaml:"confirm"`
}

// VerifySeat verifies seat position in a new hand
type VerifySeat struct {
	Seat   uint32 `yaml:"seat"`
	Player string `yaml:"player"`
}

// BotConfig contains botConfig content in the game script.
type BotConfig struct {
	MinActionPauseTime uint32 `yaml:"min-action-pause-time"`
	MaxActionPauseTime uint32 `yaml:"max-action-pause-time"`
}

/*
   - host-seat-change:
       seat-change: true
       changes:
         - seat: 1
           move: 3
         - seat: 4
           move: 5
*/

type SeatChange struct {
	Seat1 uint32
	Seat2 uint32
}
type HostSeatChange struct {
	Changes []SeatChange `yaml:"changes"`
}

type PostHandStep struct {
	HostSeatChange HostSeatChange `yaml:"host-seat-change"`
	ResumeGame     bool           `yaml:"resume-game"`
	Sleep          uint32         `yaml:"sleep"`
}

// Hand contains an entry in the hands array in the game script.
type Hand struct {
	Num           uint32         `yaml:"num"`
	Setup         HandSetup      `yaml:"setup"`
	Preflop       BettingRound   `yaml:"preflop"`
	Flop          BettingRound   `yaml:"flop"`
	Turn          BettingRound   `yaml:"turn"`
	River         BettingRound   `yaml:"river"`
	Result        HandResult     `yaml:"result"`
	PauseGame     bool           `yaml:"pause-game"`
	PostHandSteps []PostHandStep `yaml:"post-hand"`
}

// HandSetup contains the setup content in the hand config.
type HandSetup struct {
	PreDeal    []PreDealSetup       `yaml:"pre-deal"`
	ButtonPos  uint32               `yaml:"button-pos"`
	Board      []string             `yaml:"board"`
	Board2     []string             `yaml:"board2"`
	Flop       []string             `yaml:"flop"`
	Turn       string               `yaml:"turn"`
	River      string               `yaml:"river"`
	SeatCards  []SeatCards          `yaml:"seat-cards"`
	Verify     HandSetupVerfication `yaml:"verify"`
	Auto       bool                 `yaml:"auto"`
	SeatChange []SeatChangeConfirm  `yaml:"seat-change"` // players requesting seat-change
	LeaveGame  []LeaveGame          `yaml:"leave-game"`
	WaitLists  []WaitList           `yaml:"wait-list"`
	Pause      uint32               `yaml:"pause"` // bot runner pauses and waits before next hand
}

type PreDealSetup struct {
	SetupServerCrash SetupServerCrash `yaml:"setup-server-crash"`
}

type SeatCards struct {
	Cards []string `yaml:"cards"`
	Seat  uint32   `yaml:"seat"`
}

type HandSetupVerfication struct {
	Button        uint32       `yaml:"button"`
	SB            uint32       `yaml:"sb"`
	BB            uint32       `yaml:"bb"`
	NextActionPos uint32       `yaml:"next-action-pos"`
	State         string       `yaml:"state"`
	DealtCards    []SeatCards  `yaml:"dealt-cards"`
	Seats         []VerifySeat `yaml:"seats"`
	ButtonPos     *uint32      `yaml:"button-pos"`
	SBPos         *uint32      `yaml:"sb-pos"`
	BBPos         *uint32      `yaml:"bb-pos"`
}

type SeatChangeConfirm struct {
	Seat    uint32 `yaml:"seat"`
	Confirm bool   `yaml:"confirm"`
}

type LeaveGame struct {
	Seat uint32 `yaml:"seat"`
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
	Action     Action      `yaml:"action"`
	PreActions []PreAction `yaml:"pre-action"`
	Timeout    bool        `yaml:"timeout"`
}

type Action struct {
	Seat   uint32  `yaml:"seat"`
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
	a.Seat = uint32(seatNo)
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
	State        string     `yaml:"state"`
	Board        []string   `yaml:"board"`
	Ranks        []SeatRank `yaml:"ranks"`
	Pots         []Pot      `yaml:"pots"`
	NoMoreAction bool       `yaml:"no-more-action"`
}

type SeatRank struct {
	Seat    uint32 `yaml:"seat"`
	RankStr string `yaml:"rank"`
}

type Pot struct {
	Pot        float32  `yaml:"pot"`
	SeatsInPot []uint32 `yaml:"seats"`
}

type HandResult struct {
	Winners       []HandWinner   `yaml:"winners"`
	LoWinners     []HandWinner   `yaml:"lo-winners"`
	ActionEndedAt string         `yaml:"action-ended"`
	Stacks        []PlayerStack  `yaml:"stacks"`
	HighHand      []HighHandSeat `yaml:"high-hand"`
	PlayerStats   []PlayerStats  `yaml:"player-stats"`
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

type PlayerStats struct {
	Seat                      uint32 `yaml:"seat"`
	ConsecutiveActionTimeouts uint32 `yaml:"consecutive-action-timeouts"`
}

// ReadGameScript reads game script yaml file.
func ReadGameScript(fileName string) (*Script, error) {
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, errors.Wrapf(err, "Error reading game script file [%s]", fileName)
	}

	var script Script
	err = yaml.Unmarshal(bytes, &script)
	if err != nil {
		return nil, errors.Wrapf(err, "Error parsing YAML file [%s]", fileName)
	}

	err = script.Validate()
	if err != nil {
		return nil, errors.Wrapf(err, "Error validating script [%s]", fileName)
	}

	return &script, nil
}

func (s *Script) Validate() error {
	startingSeats := mapset.NewSet()
	playerNames := mapset.NewSet()

	// SOMA: I disabled this for now, since we have too many scripts to change now
	// Check max players are valid numbers.
	// validMaxPlayers := mapset.NewSet(0, 2, 4, 6, 8, 9)
	// if !validMaxPlayers.Contains(s.Game.MaxPlayers) {
	// 	return fmt.Errorf("Invalid max-players [%d]", s.Game.MaxPlayers)
	// }

	// Check starting seat numbers and player names are unique.
	for _, seat := range s.StartingSeats {
		if startingSeats.Contains(seat.Seat) {
			return fmt.Errorf("Duplicate seat number [%d] in starting-seats", seat.Seat)
		}
		startingSeats.Add(seat.Seat)
		if playerNames.Contains(seat.Player) {
			return fmt.Errorf("Duplicate player name [%s] in starting-seats", seat.Player)
		}
		playerNames.Add(seat.Player)
	}

	// Validate each hand seat numbers.
	for i, hand := range s.Hands {
		seatCardSeats := mapset.NewSet()
		validSeats := startingSeats.Clone()
		handNum := i + 1

		if hand.Setup.Auto {
			// no validation required
			continue
		}

		// Check card setup has no duplicate seat number.
		for _, seatCards := range hand.Setup.SeatCards {
			if seatCardSeats.Contains(seatCards.Seat) {
				return fmt.Errorf("Duplicate seat number [%d] in hand %d seat-cards", seatCards.Seat, handNum)
			}
			seatCardSeats.Add(seatCards.Seat)
		}

		// Check preflop seat numbers.
		for _, seatAction := range hand.Preflop.SeatActions {
			if !validSeats.Contains(seatAction.Action.Seat) {
				return fmt.Errorf("Seat number [%d] is not valid for hand %d preflop", seatAction.Action.Seat, handNum)
			}
		}

		// Check flop seat numbers.
		for _, seatAction := range hand.Flop.SeatActions {
			if !validSeats.Contains(seatAction.Action.Seat) {
				return fmt.Errorf("Seat number [%d] is not valid for hand %d flop", seatAction.Action.Seat, handNum)
			}
		}

		// Check turn seat numbers.
		for _, seatAction := range hand.Turn.SeatActions {
			if !validSeats.Contains(seatAction.Action.Seat) {
				return fmt.Errorf("Seat number [%d] is not valid for hand %d turn", seatAction.Action.Seat, handNum)
			}
		}

		// Check river seat numbers.
		for _, seatAction := range hand.River.SeatActions {
			if !validSeats.Contains(seatAction.Action.Seat) {
				return fmt.Errorf("Seat number [%d] is not valid for hand %d river", seatAction.Action.Seat, handNum)
			}
		}
	}

	return nil
}

func (s *Script) IsSeatHuman(seatNo uint32) bool {
	for _, startingSeat := range s.StartingSeats {
		if startingSeat.Seat == seatNo {
			return startingSeat.Player == s.Tester
		}
	}
	return false
}

func (s *Script) GetSeatNoByPlayerName(playerName string) uint32 {
	for _, startingSeat := range s.StartingSeats {
		if startingSeat.Player == playerName {
			return startingSeat.Seat
		}
	}
	return 0
}

func (s *Script) GetInitialBuyInAmount(seatNo uint32) float32 {
	for _, startingSeat := range s.StartingSeats {
		if startingSeat.Seat == seatNo {
			return startingSeat.BuyIn
		}
	}
	return 0
}

func (s *Script) GetSeatConfigByPlayerName(playerName string) *StartingSeat {
	for _, startingSeat := range s.StartingSeats {
		if startingSeat.Player == playerName {
			return &startingSeat
		}
	}
	return nil
}

func (s *Script) GetHand(handNum uint32) Hand {
	return s.Hands[handNum-1]
}
