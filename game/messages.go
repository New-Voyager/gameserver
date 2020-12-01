package game

type MessageDirection string

const (
	PlayerToPlayer MessageDirection = "P_2_P"
	PlayerToGame                    = "P_2_G"
	GameToPlayer                    = "G_2_P"
	GameToAll                       = "G_2_A"
)

// Game messages
const (
	GameJoin            string = "JOIN"
	GameCurrentStatus   string = "GAME_STATUS"
	PlayerTakeSeat      string = "TAKE_SEAT"
	PlayerSat           string = "PLAYER_SAT"
	GameStatusChanged   string = "STATUS_CHANGED"
	GameCurrentState    string = "GAME_STATE"
	GameQueryTableState string = "QUERY_TABLE_STATE"
	GameStart           string = "START_GAME"
	GameTableState      string = "TABLE_STATE"
	PlayerUpdate        string = "PLAYER_UPDATE"

	// These messages are used by the test driver
	GameSetupNextHand string = "SETUP_NEXT_HAND"

	// messages used internally to move the game
	GameMoveToNextHand string = "MOVE_TO_NEXT_HAND"
	GameDealHand       string = "DEAL_NEW_HAND"
)

// Hand messages
const (
	HandDeal             string = "DEAL"
	HandNewHand          string = "NEW_HAND"
	HandNextAction       string = "NEXT_ACTION"
	HandPlayerAction     string = "YOUR_ACTION"
	HandFlop             string = "FLOP"
	HandTurn             string = "TURN"
	HandRiver            string = "RIVER"
	HandShowDown         string = "SHOWDOWN"
	HandResultMessage    string = "RESULT"
	HandEnded            string = "END"
	HandPlayerActed      string = "PLAYER_ACTED"
	HandNoMoreActions    string = "NO_MORE_ACTIONS"
	HandQueryCurrentHand string = "QUERY_CURRENT_HAND"
)
