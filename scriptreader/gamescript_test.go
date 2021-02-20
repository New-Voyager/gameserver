package scriptreader

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
		Club: "Manchester Club",
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
		},
		StartingSeats: []StartingSeat{
			{
				SeatNo: 1,
				Player: "yong",
				BuyIn:  100,
			},
			{
				SeatNo: 5,
				Player: "brian",
				BuyIn:  100,
			},
			{
				SeatNo: 8,
				Player: "tom",
				BuyIn:  100,
			},
		},
		BotConfig: BotConfig{
			MinActionPauseTime: 500,
			MaxActionPauseTime: 1000,
		},
		Hands: []Hand{
			{
				Setup: HandSetup{
					ButtonPos: 1,
					Flop:      []string{"Ac", "Ad", "2c"},
					Turn:      "Td",
					River:     "4s",
					SeatCards: []SeatCards{
						{
							Cards: []string{"Kh", "Qd"},
						},
						{
							Cards: []string{"3s", "7s"},
						},
						{
							Cards: []string{"9h", "2s"},
						},
					},
					Auto: true,
					SeatChange: []SeatChange{
						{
							SeatNo:  2,
							Confirm: true,
						},
					},
					LeaveGame: []LeaveGame{
						{
							SeatNo: 6,
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
								SeatNo: 1,
								Action: "CALL",
								Amount: 2,
							},
							PreAction: PreAction{
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
								SetupServerCrash: SetupServerCrash{
									CrashPoint: "ON_PLAYER_ACTED_2",
								},
							},
						},
						{
							Action: Action{
								SeatNo: 5,
								Action: "CALL",
								Amount: 2,
							},
						},
						{
							Action: Action{
								SeatNo: 8,
								Action: "CHECK",
							},
						},
					},
				},
				Flop: BettingRound{
					Verify: BettingRoundVerification{
						Board: []string{"Ac", "Ad", "2c"},
					},
					SeatActions: []SeatAction{
						{
							Action: Action{
								SeatNo: 5,
								Action: "CHECK",
							},
						},
						{
							Action: Action{
								SeatNo: 8,
								Action: "BET",
								Amount: 2,
							},
						},
						{
							Action: Action{
								SeatNo: 1,
								Action: "CALL",
								Amount: 2,
							},
						},
						{
							Action: Action{
								SeatNo: 5,
								Action: "RAISE",
								Amount: 4,
							},
						},
						{
							Action: Action{
								SeatNo: 8,
								Action: "FOLD",
							},
						},
						{
							Action: Action{
								SeatNo: 1,
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
								SeatNo: 5,
								Action: "CHECK",
							},
						},
						{
							Action: Action{
								SeatNo: 1,
								Action: "BET",
								Amount: 10,
							},
						},
						{
							Action: Action{
								SeatNo: 5,
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
								SeatNo: 5,
								Action: "BET",
								Amount: 10,
							},
						},
						{
							Action: Action{
								SeatNo: 1,
								Action: "CALL",
								Amount: 10,
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
