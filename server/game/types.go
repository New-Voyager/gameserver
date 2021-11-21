package game

// The types used here are used by game scripts
// These test configurations/game scripts are used for automated
// testing or automated game playing

/*
    type: HOLDEM
    max-players: 9
    min-players: 3
    min-buyin: 160.0
    max-buyin: 440.0
    auto-start: true
    auto-approve: true
    title: 3 players playing
    sb: 1.0
    bb: 2.0

  players:
    - name: player1
      id: 1
    - name: player2
      id: 2
    - name: player3
      id: 3


*/
type TestGameConfig struct {
	ClubId             uint32      `json:"clubId"`
	GameId             uint64      `json:"gameId"`
	GameTypeStr        string      `yaml:"type"`
	GameType           GameType    `json:"gameType"`
	ClubCode           string      `json:"clubCode"`
	GameCode           string      `json:"gameCode"`
	Title              string      `json:"title" yaml:"title"`
	Status             GameStatus  `json:"status"`
	TableStatus        TableStatus `json:"tableStatus"`
	SmallBlind         float64     `json:"smallBlind" yaml:"sb"`
	BigBlind           float64     `json:"bigBlind" yaml:"bb"`
	StraddleBet        float64     `json:"straddleBet"`
	MinPlayers         int         `json:"minPlayers" yaml:"min-players"`
	MaxPlayers         int         `json:"maxPlayers" yaml:"max-players"`
	GameLength         int         `json:"gameLength"`
	RakePercentage     float64     `json:"rakePercentage" yaml:"rake-percentage"`
	RakeCap            float64     `json:"rakeCap" yaml:"rake-cap"`
	BuyInMin           float64     `json:"buyInMin" yaml:"min-buyin"`
	BuyInMax           float64     `json:"buyInMax" yaml:"max-buyin"`
	ActionTime         int         `json:"actionTime"`
	StartedBy          string      `json:"startedBy"`
	StartedByUuid      string      `json:"startedByUuid"`
	BreakLength        int         `json:"breakLength"`
	AutoKickAfterBreak bool        `json:"autoKickAfterBreak"`
	AutoStart          bool        `yaml:"auto-start"`
	AutoApprove        bool        `yaml:"auto-approve"`
	RewardTrackingIds  []uint32    `json:"rewardTrackingIds"`
	BringIn            float64     `json:"bringIn" yaml:"bring-in"`
}

type GamePlayer struct {
	Name       string `yaml:"name"`
	ID         uint64 `yaml:"id"`
	RunItTwice bool   `yaml:"run-it-twice"`
}

/*
take-seat:
  button-pos: 1
  seats:
    -
      seat: 1
      player: 1
      buy-in: 100
    -
      seat: 5
      player: 2
      buy-in: 100
    -
      seat: 8
      player: 2
      buy-in: 100
  wait: 1
*/
type PlayerSeat struct {
	Player                   uint64  `yaml:"player"`
	SeatNo                   uint32  `yaml:"seat"`
	BuyIn                    float64 `yaml:"buy-in"`
	RunItTwice               bool    `yaml:"run-it-twice"`
	RunItTwicePromptResponse bool    `yaml:"run-it-twice-prompt"`
	PostBlind                bool    `yaml:"post-blind"`
}

type SeatVerification struct {
	Table PokerTable `yaml:"table"`
}

type AssignSeat struct {
	ButtonPos uint32           `yaml:"button-pos"`
	Seats     []PlayerSeat     `yaml:"seats"`
	Wait      uint32           `yaml:"wait"`
	Verify    SeatVerification `yaml:"verify"`
}

/*
   button-pos: 1
   flop: ["Ac", "Ad", "2c"]
   turn: Td
   river: 3s
   player-cards:
     # note we are using the seat number, not player ids
     1: ["Kh", "Qd"]
     5: ["3s", "7s"]
     8: ["9h", "2s"]
   verify:
     # the hand setup verification is sb, bb, next-action seat, hand current state
     # player in the table' stack
     sb: 5
     bb: 8
     next-action: 1
     state: PREFLOP
	 pots:
		- seats: 1,2,3
	      pot: 77
		- seats: 1,2
		  pot: 40

*/

type HandSetupVerfication struct {
	Button        uint32          `yaml:"button"`
	SB            uint32          `yaml:"sb"`
	BB            uint32          `yaml:"bb"`
	NextActionPos uint32          `yaml:"next-action-pos"`
	State         string          `yaml:"state"`
	DealtCards    []TestSeatCards `yaml:"dealt-cards"`
	PostedBlinds  []uint32        `yaml:"posted-blinds"`
}

type TestSeatCards struct {
	Cards  []string `yaml:"cards"`
	SeatNo uint32   `yaml:"seat-no"`
}

type HandSetup struct {
	ButtonPos   uint32               `yaml:"button-pos"`
	AutoDeal    bool                 `yaml:"auto-deal"`
	Flop        []string             `yaml:"flop"`
	Turn        string               `yaml:"turn"`
	River       string               `yaml:"river"`
	Board       []string             `yaml:"board"`
	Board2      []string             `yaml:"board2"`
	SeatCards   []TestSeatCards      `yaml:"seat-cards"`
	NewPlayers  []PlayerSeat         `yaml:"new-players"`
	Verify      HandSetupVerfication `yaml:"verify"`
	BombPot     bool                 `yaml:"bomb-pot"`
	BombPotBet  uint32               `yaml:"bomb-pot-bet"`
	DoubleBoard bool                 `yaml:"double-board"`
}

/*
 actions:
	-
		seat: 1
		action: FOLD
	-
		seat: 5
		action: FOLD
*/
type TestHandAction struct {
	SeatNo       uint32        `yaml:"seat"`
	Action       string        `yaml:"action"`
	Amount       float64       `yaml:"amount"`
	VerifyAction *VerifyAction `yaml:"verify-action"`
}

type Pot struct {
	Pot        float64  `yaml:"pot"`
	SeatsInPot []uint32 `yaml:"seats"`
}

type VerifyBettingRound struct {
	State        string        `yaml:"state"`
	Board        []string      `yaml:"board"`
	Pots         []Pot         `yaml:"pots"`
	NoMoreAction bool          `yaml:"no-more-action"`
	Stacks       []PlayerStack `yaml:"stacks"`
	RunItTwice   bool          `yaml:"run-it-twice"`
}

type BettingRound struct {
	Actions     []TestHandAction   `yaml:"actions"`
	SeatActions []string           `yaml:"seat-actions"`
	Verify      VerifyBettingRound `yaml:"verify"`
}

/*
   result:
     winners:
       -
         seat: 8
         receive: 3.0
     action-ended: PREFLOP

     # balance indicates the player balance after the hand
     stacks:
       -
         seat: 1
         stack: 150
       -
         player: 5
         stack: 99
       -
         player: 8
         stack: 101
*/
type TestHandWinner struct {
	Seat    uint32  `yaml:"seat"`
	Receive float64 `yaml:"receive"`
	RankStr string  `yaml:"rank"`
	Rake    float64 `yaml:"rake"`
}

type PlayerStack struct {
	Seat  uint32  `yaml:"seat"`
	Stack float64 `yaml:"stack"`
}

type TestHandResultVerify struct {
	Winners       []TestHandWinner `yaml:"winners"`
	LoWinners     []TestHandWinner `yaml:"lo-winners"`
	ActionEndedAt string           `yaml:"action-ended"`
	Stacks        []PlayerStack    `yaml:"stacks"`
}

type TestHandResult struct {
	TestHandResultVerify
	Boards []TestHandResultVerify `yaml:"boards"`
}
type Hand struct {
	Num           uint32         `yaml:"num"`
	Setup         HandSetup      `yaml:"setup"`
	PreflopAction BettingRound   `yaml:"preflop-action"`
	FlopAction    BettingRound   `yaml:"flop-action"`
	TurnAction    BettingRound   `yaml:"turn-action"`
	RiverAction   BettingRound   `yaml:"river-action"`
	Result        TestHandResult `yaml:"result"`
}

type GameScript struct {
	Disabled   bool           `yaml:"disabled"`
	Hands      []Hand         `yaml:"hands"`
	Players    []GamePlayer   `yaml:"players"`
	AssignSeat AssignSeat     `yaml:"take-seat"`
	GameConfig TestGameConfig `yaml:"game-config"`
}

type PlayerAtTable struct {
	SeatNo   uint32  `yaml:"seat"`
	PlayerID uint64  `yaml:"player"`
	Stack    float64 `yaml:"stack"`
}

type PokerTable struct {
	Players []PlayerAtTable `yaml:"players"`
}

type BetAmount struct {
	Text   string  `yaml:"text"`
	Amount float64 `yaml:"amount"`
}

type VerifyAction struct {
	Actions        []string    `yaml:"actions"`
	CallAmount     float64     `yaml:"call-amount"`
	AllInAmount    float64     `yaml:"all-in-amount"`
	MinRaiseAmount float64     `yaml:"min-raise-amount"`
	MaxRaiseAmount float64     `yaml:"max-raise-amount"`
	BetAmounts     []BetAmount `yaml:"bet-amounts"`
}

type HighHandResult struct {
	GameCode         string   `json:"gameCode"`
	HandNum          int      `json:"handNum"`
	RewardTrackingID int      `json:"rewardTrackingId"`
	AssociatedGames  []string `json:"associatedGames"`
	Winners          []struct {
		GameCode    string `json:"gameCode"`
		PlayerID    string `json:"playerId"`
		PlayerUUID  string `json:"playerUuid"`
		PlayerName  string `json:"playerName"`
		BoardCards  []int  `json:"boardCards"`
		PlayerCards []int  `json:"playerCards"`
		HhCards     []int  `json:"hhCards"`
	} `json:"winners"`
}

type SaveHandResult struct {
	GameCode string          `json:"gameCode"`
	HandNum  int             `json:"handNum"`
	Success  bool            `json:"success"`
	HighHand *HighHandResult `json:"highHand"`
}

type SeatPlayer struct {
	SeatNo             uint32
	OpenSeat           bool
	PlayerID           uint64 `json:"playerId"`
	PlayerUUID         string `json:"playerUuid"`
	Name               string
	EncryptionKey      string
	BuyIn              float64
	Stack              float64
	Status             PlayerStatus
	GameToken          string
	GameTokenInt       uint64
	BuyInTimeExpAt     string
	BreakTimeExpAt     string
	PostedBlind        bool
	Inhand             bool
	RunItTwice         bool
	MissedBlind        bool
	MuckLosingHand     bool
	ActiveSeat         bool
	AutoStraddle       bool
	ButtonStraddle     bool
	ButtontStraddleBet uint32 `json:"buttonStraddleBet"` // multiples of big blind
}

/*
export interface NewHandInfo {
  gameCode: string;
  gameType: GameType;
  maxPlayers: number;
  smallBlind: number;
  bigBlind: number;
  buttonPos: number;
  announceGameType: boolean;
  playersInSeats: Array<PlayerInSeat>;
}*/
type NewHandInfo struct {
	GameID            uint64 `json:"gameId"`
	GameCode          string
	GameType          GameType
	MaxPlayers        uint32
	SmallBlind        float64
	BigBlind          float64
	ButtonPos         uint32
	HandNum           uint32
	ActionTime        uint32
	StraddleBet       float64
	ChipUnit          ChipUnit
	RakePercentage    float64
	RakeCap           float64
	AnnounceGameType  bool
	PlayersInSeats    []SeatPlayer
	GameStatus        GameStatus
	TableStatus       TableStatus
	SbPos             uint32
	BbPos             uint32
	ResultPauseTime   uint32
	BombPot           bool
	DoubleBoard       bool
	BombPotBet        float64
	BringIn           float64
	RunItTwiceTimeout uint32
	HighHandRank      uint32
	HighHandTracked   bool
}
