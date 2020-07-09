package test

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
type GameConfig struct {
	GameType    string  `yaml:"type"`
	MinPlayers  uint32  `yaml:"min-players"`
	MaxPlayers  uint32  `yaml:"max-players"`
	MinBuyin    float32 `yaml:"min-buyin"`
	MaxBuyin    float32 `yaml:"max-buyin"`
	AutoStart   bool    `yaml:"auto-start"`
	AutoApprove bool    `yaml:"auto-approve"`
	Title       string  `yaml:"title"`
	SB          float32 `yaml:"sb"`
	BB          float32 `yaml:"bb"`
}

type GamePlayer struct {
	Name string `yaml:"name"`
	ID   uint32 `yaml:"id"`
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
	Player uint32  `yaml:"player"`
	SeatNo uint32  `yaml:"seat"`
	BuyIn  float32 `yaml:"buy-in"`
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
*/

type HandSetupVerfication struct {
	Button        uint32      `yaml:"button"`
	SB            uint32      `yaml:"sb"`
	BB            uint32      `yaml:"bb"`
	NextActionPos uint32      `yaml:"next-action-pos"`
	State         string      `yaml:"state"`
	DealtCards    []SeatCards `yaml:"dealt-cards"`
}

type SeatCards struct {
	Cards  []string `yaml:"cards"`
	SeatNo uint32   `yaml:"seat-no"`
}

type HandSetup struct {
	ButtonPos uint32               `yaml:"button-pos"`
	Flop      []string             `yaml:"flop"`
	Turn      string               `yaml:"turn"`
	River     string               `yaml:"river"`
	SeatCards []SeatCards          `yaml:"seat-cards"`
	Verify    HandSetupVerfication `yaml:"verify"`
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
type HandAction struct {
	SeatNo uint32  `yaml:"seat"`
	Action string  `yaml:"action"`
	Amount float32 `yaml:"amount"`
}

type VerifyBettingRound struct {
	State string `yaml:"state"`
}

type BettingRound struct {
	Actions []HandAction       `yaml:"actions"`
	Verify  VerifyBettingRound `yaml:"verify"`
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
type HandWinner struct {
	Seat    uint32  `yaml:"seat"`
	Receive float32 `yaml:"receive"`
}

type PlayerStack struct {
	Seat  uint32  `yaml:"seat"`
	Stack float32 `yaml:"stack"`
}

type HandResult struct {
	Winners       []HandWinner  `yaml:"winners"`
	ActionEndedAt string        `yaml:"action-ended"`
	Stacks        []PlayerStack `yaml:"stacks"`
}

type Hand struct {
	Num           uint32       `yaml:"num"`
	Setup         HandSetup    `yaml:"setup"`
	PreflopAction BettingRound `yaml:"preflop-action"`
	Result        HandResult   `yaml:"result"`

	gameScript *GameScript
}

type GameScript struct {
	Disabled   bool         `yaml:"disabled"`
	Hands      []Hand       `yaml:"hands"`
	Players    []GamePlayer `yaml:"players"`
	AssignSeat AssignSeat   `yaml:"take-seat"`
	GameConfig GameConfig   `yaml:"game-config"`

	testGame *TestGame
	filename string
	result   *ScriptTestResult
	observer *TestPlayer
}

type PlayerAtTable struct {
	SeatNo   uint32  `yaml:"seat"`
	PlayerID uint32  `yaml:"player"`
	Stack    float32 `yaml:"stack"`
}

type PokerTable struct {
	Players []PlayerAtTable `yaml:"players"`
}
