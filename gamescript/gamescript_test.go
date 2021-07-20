package gamescript

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReadGameScript(t *testing.T) {
	script, err := ReadGameScript("test_scripts/script1.yaml")
	if err != nil {
		t.Fatalf("ReadGameScript returned error [%s]", err)
	}
	if script == nil {
		t.Fatal("ReadGameScript returned nil data")
	}

	expectedScript := Script{
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
			RakePercentage:     5.0,
			RakeCap:            5.0,
			BuyInMin:           100,
			BuyInMax:           300,
			ActionTime:         100,
			Rewards:            "High Hand",
			RoeGames:           []string{"HOLDEM", "PLO"},
		},
		StartingSeats: []StartingSeat{
			{
				Seat:           1,
				Player:         "yong",
				BuyIn:          100,
				Reload:         getBoolPointer(true),
				MuckLosingHand: true,
				PostBlind:      true,
			},
			{
				Seat:           5,
				Player:         "brian",
				BuyIn:          100,
				Reload:         getBoolPointer(false),
				MuckLosingHand: false,
				PostBlind:      false,
			},
			{
				Seat:           8,
				Player:         "tom",
				BuyIn:          100,
				Reload:         nil,
				MuckLosingHand: false,
				PostBlind:      false,
			},
		},
		Tester:   "tom",
		AutoPlay: true,
		BotConfig: BotConfig{
			MinActionPauseTime: 500,
			MaxActionPauseTime: 1000,
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
							},
							{
								Seat:   2,
								Player: "brian",
							},
							{
								Seat:   3,
								Player: "yong",
							},
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
										StraddleAmount:   3,
										CallAmount:       5,
										RaiseAmount:      10,
										MinBetAmount:     2,
										MaxBetAmount:     4,
										MinRaiseAmount:   10,
										MaxRaiseAmount:   30,
										AllInAmount:      200,
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
						},
						{
							Action: Action{
								Seat:   8,
								Action: "CHECK",
							},
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
								RankStr: "Pair",
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
								Before: getFloat32Pointer(100),
								After:  getFloat32Pointer(84),
							},
						},
						{
							Seat:   5,
							HhRank: getUint32Pointer(2255),
							Balance: PlayerBalance{
								After: getFloat32Pointer(120),
							},
						},
						{
							Seat:   8,
							HhRank: nil,
							Balance: PlayerBalance{
								After: getFloat32Pointer(96),
							},
						},
					},
					HighHand: []HighHandSeat{
						{
							Seat: 1,
						},
					},
					PlayerStats: []PlayerStats{
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
					RunItTwice: &RunItTwiceResult{
						ShouldBeNull: true,
						StartedAt:    "FLOP",
						Board1Winners: []WinnerPot{
							{
								Amount: 126,
								Winners: []HandWinner{
									{
										Seat:    3,
										Receive: 96,
										RankStr: "Two Pair",
									},
								},
								LoWinners: []HandWinner{
									{
										Seat:    2,
										Receive: 30,
									},
								},
							},
						},
						Board2Winners: []WinnerPot{
							{
								Amount: 121,
								Winners: []HandWinner{
									{
										Seat:    2,
										Receive: 101,
										RankStr: "Pair",
									},
								},
								LoWinners: []HandWinner{
									{
										Seat:    3,
										Receive: 20,
									},
								},
							},
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

func getFloat32Pointer(v float32) *float32 {
	return &v
}

func getBoolPointer(v bool) *bool {
	return &v
}
