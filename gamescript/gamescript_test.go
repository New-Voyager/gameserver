package gamescript

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReadGameScript(t *testing.T) {
	script, err := ReadGameScript("test_scripts/all-fields.yaml")
	if err != nil {
		t.Fatalf("ReadGameScript returned error [%s]", err)
	}
	if script == nil {
		t.Fatal("ReadGameScript returned nil data")
	}

	expectedScript := Script{
		ServerSettings: &ServerSettings{
			GameBlockTime:        30,
			NotifyHostTimeWindow: 10,
			GameCoinsPerBlock:    3,
			FreeTime:             30,
		},
		Club: Club{
			Name:        "Bad Robots",
			Description: "Club for testing",
			Rewards: []Reward{
				{
					Name:     "High Hand",
					Type:     "HIGH_HAND",
					Amount:   100,
					Schedule: "ENTIRE_GAME",
				},
			},
		},
		Game: Game{
			Create:             true,
			Title:              "NLH Testing Game",
			GameType:           "HOLDEM",
			SmallBlind:         1.0,
			BigBlind:           2.0,
			UtgStraddleAllowed: true,
			StraddleBet:        4.0,
			MinPlayers:         2,
			MaxPlayers:         9,
			GameLength:         60,
			BuyInApproval:      true,
			ChipUnit:           "CENT",
			RakePercentage:     5.0,
			RakeCap:            5.0,
			BuyInMin:           100,
			BuyInMax:           300,
			ActionTime:         100,
			Rewards:            "High Hand",
			RoeGames:           []string{"HOLDEM", "PLO"},
			DealerChoiceGames:  []string{"HOLDEM", "PLO", "PLO_HILO", "FIVE_CARD_PLO", "FIVE_CARD_PLO_HILO"},
			DealerChoiceOrbit:  true,
		},
		StartingSeats: []StartingSeat{
			{
				Seat:           1,
				Player:         "yong",
				BuyIn:          100,
				AutoReload:     getBoolPointer(true),
				MuckLosingHand: true,
				PostBlind:      true,
			},
			{
				Seat:           5,
				Player:         "brian",
				BuyIn:          100,
				AutoReload:     getBoolPointer(false),
				MuckLosingHand: false,
				PostBlind:      false,
			},
			{
				Seat:           8,
				Player:         "tom",
				BuyIn:          100,
				AutoReload:     nil,
				MuckLosingHand: false,
				PostBlind:      false,
			},
		},
		Tester: "tom",
		AutoPlay: AutoPlay{
			Enabled:      true,
			HandsPerGame: 250,
			NumGames:     10,
		},
		BotConfig: BotConfig{
			MinActionDelay: 500,
			MaxActionDelay: 1000,
		},
		AfterGame: AfterGame{
			Verify: AfterGameVerification{
				NumHandsPlayed: NumHandsPlayedVerification{
					Gte: getUint32Pointer(2),
					Lte: getUint32Pointer(3),
				},
				GameMessages: []NonProtoMessage{
					{
						Type:       "PLAYER_SEAT_CHANGE_PROMPT",
						PlayerName: "tom",
						OpenedSeat: 5,
						PromptSecs: 30,
					},
					{
						Type:    "TABLE_UPDATE",
						SubType: "HostSeatChangeMove",
						SeatMoves: []SeatUpdate{
							{
								OldSeatNo: 4,
								NewSeatNo: 1,
								OpenSeat:  true,
							},
							{
								Name:      "yong",
								OldSeatNo: 1,
								NewSeatNo: 4,
								OpenSeat:  false,
							},
						},
					},
					{
						Type: "NEW_HIGHHAND_WINNER",
						Winners: []HighHandWinner{
							{
								PlayerName:  "brian",
								BoardCards:  []uint32{52, 49, 50, 17, 4},
								PlayerCards: []uint32{56, 72},
								HhCards:     []uint32{52, 49, 50, 56, 72},
							},
						},
					},
				},
				PrivateMessages: []VerifyPrivateMessages{
					{
						Player: "yong",
						Messages: []PrivateMessage{
							{
								Type: "APPCOIN_NEEDED",
							},
						},
					},
					{
						Player: "tom",
						Messages: []PrivateMessage{
							{
								Type: "YOUR_ACTION",
							},
						},
					},
				},
				APIVerification: APIVerification{
					GameResultTable: []GameResultTableRow{
						{
							PlayerName:  "yong",
							HandsPlayed: 2,
							BuyIn:       100,
							Profit:      -2,
							Stack:       98,
							RakePaid:    0,
						},
						{
							PlayerName:  "tom",
							HandsPlayed: 2,
							BuyIn:       200,
							Profit:      4,
							Stack:       204,
							RakePaid:    1,
						},
					},
				},
			},
		},
		Hands: []Hand{
			{
				Setup: HandSetup{
					PreDeal: []PreDealSetup{
						{
							SetupServerCrash: SetupServerCrash{
								CrashPoint: "DEAL_1",
							},
						},
					},
					ButtonPos: 1,
					Flop:      []string{"Ac", "Ad", "2c"},
					Turn:      "Td",
					River:     "4s",
					SeatCards: []SeatCards{
						{
							Seat:  1,
							Cards: []string{"Kh", "Qd"},
						},
						{
							Seat:  5,
							Cards: []string{"3s", "7s"},
						},
						{
							Seat:  8,
							Cards: []string{"9h", "2s"},
						},
					},
					Auto: true,
					SeatChange: []SeatChangeSetup{
						{
							Seat:    2,
							Confirm: true,
						},
					},
					RunItTwice: []RunItTwiceSetup{
						{
							Seat:        2,
							AllowPrompt: true,
							Confirm:     true,
							Timeout:     false,
						},
						{
							Seat:        3,
							AllowPrompt: true,
							Confirm:     true,
							Timeout:     true,
						},
					},
					TakeBreak: []TakeBreakSetup{
						{
							Seat: 5,
						},
						{
							Seat: 7,
						},
					},
					SitBack: []SitBackSetup{
						{
							Seat: 5,
						},
						{
							Seat: 7,
						},
					},

					LeaveGame: []LeaveGame{
						{
							Seat: 6,
						},
					},
					WaitLists: []WaitList{
						{
							Player:  "david",
							Confirm: true,
							BuyIn:   500,
						},
					},
					Pause: 5,
					Verify: HandSetupVerfication{
						GameType:      "HOLDEM",
						ButtonPos:     getUint32Pointer(1),
						SBPos:         getUint32Pointer(2),
						BBPos:         getUint32Pointer(3),
						NextActionPos: getUint32Pointer(4),
						Seats: []VerifySeat{
							{
								Seat:   1,
								Player: "tom",
								InHand: getBoolPointer(true),
								Button: getBoolPointer(true),
								Bb:     getBoolPointer(true),
								Stack:  getFloat64Pointer(29.99),
							},
							{
								Seat:   2,
								Player: "brian",
								Status: "IN_BREAK",
								InHand: getBoolPointer(false),
								Stack:  getFloat64Pointer(30.01),
							},
							{
								Seat:   3,
								Player: "yong",
								Sb:     getBoolPointer(true),
							},
						},
					},
				},
				WhenNotEnoughPlayers: WhenNotEnoughPlayers{
					RequestEndGame: true,
					AddPlayers: []StartingSeat{
						{
							Seat:   2,
							Player: "jim",
							BuyIn:  100,
						},
					},
				},
				Preflop: BettingRound{
					SeatActions: []SeatAction{
						{
							Action: Action{
								Seat:   1,
								Action: "CALL",
								Amount: 2,
							},
							PreActions: []PreAction{
								{
									SetupServerCrash: SetupServerCrash{
										CrashPoint: "ON_PLAYER_ACTED_2",
									},
								},
								{
									Verify: YourActionVerification{
										AvailableActions: []string{"FOLD", "CALL", "RAISE", "ALLIN"},
										StraddleAmount:   getFloat64Pointer(3),
										CallAmount:       getFloat64Pointer(5),
										RaiseAmount:      getFloat64Pointer(10),
										MinBetAmount:     getFloat64Pointer(2),
										MaxBetAmount:     getFloat64Pointer(4),
										MinRaiseAmount:   getFloat64Pointer(10),
										MaxRaiseAmount:   getFloat64Pointer(30),
										AllInAmount:      getFloat64Pointer(200),
										BetOptions: []BetOption{
											{
												Text:   "Pot",
												Amount: 100,
											},
											{
												Text:   "All-In",
												Amount: 300,
											},
										},
									},
								},
							},
						},
						{
							Action: Action{
								Seat:   5,
								Action: "CALL",
								Amount: 2,
							},
							Verify: &VerifyAction{
								Stack:      100,
								PotUpdates: 10,
							},
						},
						{
							Action: Action{
								Seat:   8,
								Action: "CHECK",
							},
							Timeout:            true,
							ActionDelay:        10000,
							ExtendTimeoutBySec: 10,
							ResetTimerToSec:    20,
						},
					},
				},
				Flop: BettingRound{
					Verify: BettingRoundVerification{
						Board: []string{"Ac", "Ad", "2c"},
						Ranks: []SeatRank{
							{
								Seat:    1,
								RankStr: "Two Pair",
							},
							{
								Seat:    5,
								RankStr: "Pair,Two Pair",
							},
						},
					},
					SeatActions: []SeatAction{
						{
							Action: Action{
								Seat:   5,
								Action: "CHECK",
							},
						},
						{
							Action: Action{
								Seat:   8,
								Action: "BET",
								Amount: 2,
							},
						},
						{
							Action: Action{
								Seat:   1,
								Action: "CALL",
								Amount: 2,
							},
						},
						{
							Action: Action{
								Seat:   5,
								Action: "RAISE",
								Amount: 4,
							},
						},
						{
							Action: Action{
								Seat:   8,
								Action: "FOLD",
							},
						},
						{
							Action: Action{
								Seat:   1,
								Action: "CALL",
								Amount: 4,
							},
						},
					},
				},
				Turn: BettingRound{
					Verify: BettingRoundVerification{
						Board: []string{"Ac", "Ad", "2c", "Td"},
					},
					SeatActions: []SeatAction{
						{
							Action: Action{
								Seat:   5,
								Action: "CHECK",
							},
						},
						{
							Action: Action{
								Seat:   1,
								Action: "BET",
								Amount: 10,
							},
						},
						{
							Action: Action{
								Seat:   5,
								Action: "CALL",
								Amount: 10,
							},
						},
					},
				},
				River: BettingRound{
					Verify: BettingRoundVerification{
						Board: []string{"Ac", "Ad", "2c", "Td", "4s"},
					},
					SeatActions: []SeatAction{
						{
							Action: Action{
								Seat:   5,
								Action: "BET",
								Amount: 10,
							},
						},
						{
							Action: Action{
								Seat:   1,
								Action: "CALL",
								Amount: 10,
							},
						},
					},
				},
				Result: HandResult{
					Winners: []HandWinner{
						{
							Seat:    1,
							Receive: 56.0,
							RankStr: "Two Pair",
						},
						{
							Seat:    5,
							Receive: 12.0,
						},
					},
					LoWinners: []HandWinner{
						{
							Seat:    2,
							Receive: 12.0,
							RankStr: "Pair",
						},
						{
							Seat:    3,
							Receive: 13.0,
						},
					},
					ActionEndedAt: "SHOW_DOWN",
					Players: []ResultPlayer{
						{
							Seat:   1,
							HhRank: getUint32Pointer(127),
							Balance: PlayerBalance{
								Before: getFloat64Pointer(100),
								After:  getFloat64Pointer(84),
							},
							PotContribution: getFloat64Pointer(37.44),
						},
						{
							Seat:   5,
							HhRank: getUint32Pointer(2255),
							Balance: PlayerBalance{
								After: getFloat64Pointer(120),
							},
							PotContribution: getFloat64Pointer(5.61),
						},
						{
							Seat:   8,
							HhRank: nil,
							Balance: PlayerBalance{
								After: getFloat64Pointer(96),
							},
						},
					},
					TipsCollected: getFloat64Pointer(4.96),
					TimeoutStats: []TimeoutStats{
						{
							Seat:                      1,
							ConsecutiveActionTimeouts: 0,
							ActedAtLeastOnce:          true,
						},
						{
							Seat:                      5,
							ConsecutiveActionTimeouts: 3,
							ActedAtLeastOnce:          true,
						},
						{
							Seat:                      8,
							ConsecutiveActionTimeouts: 1,
							ActedAtLeastOnce:          false,
						},
					},
				},
				APIVerification: APIVerification{
					GameResultTable: []GameResultTableRow{
						{
							PlayerName:  "yong",
							HandsPlayed: 1,
							BuyIn:       100,
							Profit:      -2,
							Stack:       98,
							RakePaid:    0,
						},
						{
							PlayerName:  "tom",
							HandsPlayed: 1,
							BuyIn:       100,
							Profit:      2,
							Stack:       102,
							RakePaid:    1,
						},
					},
				},
			},
		},
	}

	if !cmp.Equal(*script, expectedScript) {
		t.Errorf(cmp.Diff(*script, expectedScript))
	}
}

func getUint32Pointer(v uint32) *uint32 {
	return &v
}

func getFloat64Pointer(v float64) *float64 {
	return &v
}

func getBoolPointer(v bool) *bool {
	return &v
}
