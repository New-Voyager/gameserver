package crashtest

import (
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
	"voyager.com/server/internal"
)

// CrashPoint is an enum representing different points in the code that the server can crash.
type CrashPoint string

const (
	CrashPoint_NO_CRASH               CrashPoint = "NO_CRASH"
	CrashPoint_NOW                    CrashPoint = "NOW"
	CrashPoint_DEAL_1                 CrashPoint = "DEAL_1"
	CrashPoint_DEAL_2                 CrashPoint = "DEAL_2"
	CrashPoint_DEAL_3                 CrashPoint = "DEAL_3"
	CrashPoint_DEAL_4                 CrashPoint = "DEAL_4"
	CrashPoint_DEAL_5                 CrashPoint = "DEAL_5"
	CrashPoint_DEAL_6                 CrashPoint = "DEAL_6"
	CrashPoint_WAIT_FOR_NEXT_ACTION_1 CrashPoint = "WAIT_FOR_NEXT_ACTION_1"
	CrashPoint_WAIT_FOR_NEXT_ACTION_2 CrashPoint = "WAIT_FOR_NEXT_ACTION_2"
	CrashPoint_PREPARE_NEXT_ACTION_1  CrashPoint = "PREPARE_NEXT_ACTION_1"
	CrashPoint_PREPARE_NEXT_ACTION_2  CrashPoint = "PREPARE_NEXT_ACTION_2"
	CrashPoint_PREPARE_NEXT_ACTION_3  CrashPoint = "PREPARE_NEXT_ACTION_3"
	CrashPoint_PREPARE_NEXT_ACTION_4  CrashPoint = "PREPARE_NEXT_ACTION_4"
	CrashPoint_MOVE_TO_NEXT_ACTION_1  CrashPoint = "MOVE_TO_NEXT_ACTION_1"
	CrashPoint_MOVE_TO_NEXT_ACTION_2  CrashPoint = "MOVE_TO_NEXT_ACTION_2"
	CrashPoint_MOVE_TO_NEXT_ACTION_3  CrashPoint = "MOVE_TO_NEXT_ACTION_3"
	CrashPoint_MOVE_TO_NEXT_ACTION_4  CrashPoint = "MOVE_TO_NEXT_ACTION_4"
	CrashPoint_MOVE_TO_NEXT_ROUND_1   CrashPoint = "MOVE_TO_NEXT_ROUND_1"
	CrashPoint_MOVE_TO_NEXT_ROUND_2   CrashPoint = "MOVE_TO_NEXT_ROUND_2"
	CrashPoint_MOVE_TO_NEXT_ROUND_3   CrashPoint = "MOVE_TO_NEXT_ROUND_3"
	CrashPoint_ALL_PLAYERS_ALL_IN_1   CrashPoint = "ALL_PLAYERS_ALL_IN_1"
	CrashPoint_ALL_PLAYERS_ALL_IN_2   CrashPoint = "ALL_PLAYERS_ALL_IN_2"
	CrashPoint_ALL_PLAYERS_ALL_IN_3   CrashPoint = "ALL_PLAYERS_ALL_IN_3"
	CrashPoint_ALL_PLAYERS_ALL_IN_4   CrashPoint = "ALL_PLAYERS_ALL_IN_4"
	CrashPoint_ALL_PLAYERS_ALL_IN_5   CrashPoint = "ALL_PLAYERS_ALL_IN_5"
	CrashPoint_ALL_PLAYERS_ALL_IN_6   CrashPoint = "ALL_PLAYERS_ALL_IN_6"
	CrashPoint_ALL_PLAYERS_ALL_IN_7   CrashPoint = "ALL_PLAYERS_ALL_IN_7"
	CrashPoint_ALL_PLAYERS_ALL_IN_8   CrashPoint = "ALL_PLAYERS_ALL_IN_8"
	CrashPoint_MOVE_TO_NEXT_HAND_1    CrashPoint = "MOVE_TO_NEXT_HAND_1"
	CrashPoint_MOVE_TO_NEXT_HAND_2    CrashPoint = "MOVE_TO_NEXT_HAND_2"
	CrashPoint_MOVE_TO_NEXT_HAND_3    CrashPoint = "MOVE_TO_NEXT_HAND_3"
	CrashPoint_MOVE_TO_NEXT_HAND_4    CrashPoint = "MOVE_TO_NEXT_HAND_4"
	CrashPoint_MOVE_TO_NEXT_HAND_5    CrashPoint = "MOVE_TO_NEXT_HAND_5"

	ExitCode int = 66
)

// IsValid checks if cp is a valid enum value for CrashPoint.
func (cp CrashPoint) IsValid() bool {
	switch cp {
	case CrashPoint_NO_CRASH, CrashPoint_NOW, CrashPoint_DEAL_1, CrashPoint_DEAL_2, CrashPoint_DEAL_3, CrashPoint_DEAL_4, CrashPoint_DEAL_5, CrashPoint_DEAL_6, CrashPoint_WAIT_FOR_NEXT_ACTION_1, CrashPoint_WAIT_FOR_NEXT_ACTION_2, CrashPoint_PREPARE_NEXT_ACTION_1, CrashPoint_PREPARE_NEXT_ACTION_2, CrashPoint_PREPARE_NEXT_ACTION_3, CrashPoint_PREPARE_NEXT_ACTION_4, CrashPoint_MOVE_TO_NEXT_ACTION_1, CrashPoint_MOVE_TO_NEXT_ACTION_2, CrashPoint_MOVE_TO_NEXT_ACTION_3, CrashPoint_MOVE_TO_NEXT_ACTION_4, CrashPoint_MOVE_TO_NEXT_ROUND_1, CrashPoint_MOVE_TO_NEXT_ROUND_2, CrashPoint_MOVE_TO_NEXT_ROUND_3, CrashPoint_ALL_PLAYERS_ALL_IN_1, CrashPoint_ALL_PLAYERS_ALL_IN_2, CrashPoint_ALL_PLAYERS_ALL_IN_3, CrashPoint_ALL_PLAYERS_ALL_IN_4, CrashPoint_ALL_PLAYERS_ALL_IN_5, CrashPoint_ALL_PLAYERS_ALL_IN_6, CrashPoint_ALL_PLAYERS_ALL_IN_7, CrashPoint_ALL_PLAYERS_ALL_IN_8, CrashPoint_MOVE_TO_NEXT_HAND_1, CrashPoint_MOVE_TO_NEXT_HAND_2, CrashPoint_MOVE_TO_NEXT_HAND_3, CrashPoint_MOVE_TO_NEXT_HAND_4, CrashPoint_MOVE_TO_NEXT_HAND_5:
		return true
	}
	return false
}

var (
	crashGameCode string
	crashPoint    CrashPoint = CrashPoint_NO_CRASH
	crashPlayerID uint64

	crashTestLogger = log.With().Str("logger_name", "crashtest::crashtest").Logger()
)

// Set schedules for crashing at the specified point.
// If cp == CrashPoint_NOW, the function will crash immediately without returning.
func Set(gameCode string, cp CrashPoint, playerID uint64) error {
	if !cp.IsValid() {
		return fmt.Errorf("Invalid crash point enum: [%s]", cp)
	}
	crashGameCode = gameCode
	crashPoint = cp
	crashPlayerID = playerID
	if cp == CrashPoint_NOW {
		Hit(gameCode, cp, playerID)
	}
	return nil
}

// Hit will crash the process if cp matches the crash point scheduled by Set.
func Hit(gameCode string, cp CrashPoint, playerID uint64) {
	if cp != crashPoint || playerID != crashPlayerID {
		return
	}
	// Save to the crash tracking trable.
	err := saveToTracker(gameCode, cp)
	if err != nil {
		fmt.Printf("Error while inserting crash record. Game Code: %s, Crash Point: %s, Error: %s\n", gameCode, cp, err)
	} else {
		fmt.Printf("CRASHTEST (This is an intentional crash) GameCode: %s CrashPoint: %s, CrashPlayerID: %d\n", gameCode, cp, crashPlayerID)
	}
	os.Exit(ExitCode)
}

func saveToTracker(gameCode string, cp CrashPoint) error {
	db := sqlx.MustConnect("postgres", internal.GetGamesConnStr())
	defer db.Close()
	result := db.MustExec("INSERT INTO crash_test (game_code, crash_point) VALUES ($1, $2)", gameCode, string(cp))
	numRows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if numRows != 1 {
		return fmt.Errorf("Rows inserted != 1")
	}
	return nil
}
