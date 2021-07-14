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
		},
		StartingSeats: []StartingSeat{
			{
				Seat:   1,
				Player: "yong",
				BuyIn:  100,
			},
			{
				Seat:   5,
				Player: "brian",
				BuyIn:  100,
			},
			{
				Seat:   8,
				Player: "tom",
				BuyIn:  100,
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
					SeatChange: []SeatChangeConfirm{
						{
							Seat:    2,
							Confirm: true,
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
					ActionEndedAt: "SHOW_DOWN",
					Players: []ResultPlayer{
						{
							Seat:   1,
							HhRank: 127,
							Balance: PlayerBalance{
								After: 84,
							},
						},
						{
							Seat:   5,
							HhRank: 2255,
							Balance: PlayerBalance{
								After: 120,
							},
						},
						{
							Seat:   8,
							HhRank: 0,
							Balance: PlayerBalance{
								After: 96,
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
				},
			},
		},
	}

	if !cmp.Equal(*script, expectedScript) {
		t.Errorf(cmp.Diff(*script, expectedScript))
	}
}
