package crashtest

import (
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
	"voyager.com/server/util"
)

// CrashPoint is an enum representing different points in the code that the server can crash.
type CrashPoint string

const (
	CrashPoint_NO_CRASH               CrashPoint = "NO_CRASH"
	CrashPoint_NOW                    CrashPoint = "NOW"
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

	ExitCode int = 66
)

// IsValid checks if cp is a valid enum value for CrashPoint.
func (cp CrashPoint) IsValid() bool {
	switch cp {
	case CrashPoint_NO_CRASH, CrashPoint_NOW, CrashPoint_WAIT_FOR_NEXT_ACTION_1, CrashPoint_WAIT_FOR_NEXT_ACTION_2, CrashPoint_PREPARE_NEXT_ACTION_1, CrashPoint_PREPARE_NEXT_ACTION_2, CrashPoint_PREPARE_NEXT_ACTION_3, CrashPoint_PREPARE_NEXT_ACTION_4, CrashPoint_MOVE_TO_NEXT_ACTION_1, CrashPoint_MOVE_TO_NEXT_ACTION_2, CrashPoint_MOVE_TO_NEXT_ACTION_3, CrashPoint_MOVE_TO_NEXT_ACTION_4:
		return true
	}
	return false
}

var (
	crashGameCode string
	crashPoint    CrashPoint = CrashPoint_NO_CRASH

	crashTestLogger = log.With().Str("logger_name", "crashtest::crashtest").Logger()
)

// Set schedules for crashing at the specified point.
// If cp == CrashPoint_NOW, the function will crash immediately without returning.
func Set(gameCode string, cp CrashPoint) error {
	if !cp.IsValid() {
		return fmt.Errorf("Invalid crash point enum: [%s]", cp)
	}
	crashGameCode = gameCode
	crashPoint = cp
	if cp == CrashPoint_NOW {
		Hit(gameCode, cp)
	}
	return nil
}

// Hit will crash the process if cp matches the crash point scheduled by Set.
func Hit(gameCode string, cp CrashPoint) {
	if cp != crashPoint {
		return
	}
	fmt.Printf("CRASHTEST: %s %s\n", gameCode, cp)
	// Save to the crash tracking trable.
	err := saveToTracker(gameCode, cp)
	if err != nil {
		fmt.Printf("Error while inserting crash record. Game Code: %s, Crash Point: %s, Error: %s\n", gameCode, cp, err)
	}
	os.Exit(ExitCode)
}

func saveToTracker(gameCode string, cp CrashPoint) error {
	db := sqlx.MustConnect("postgres", getConnStr())
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

func getConnStr() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		util.GameServerEnvironment.GetPostgresHost(),
		util.GameServerEnvironment.GetPostgresPort(),
		util.GameServerEnvironment.GetPostgresUser(),
		util.GameServerEnvironment.GetPostgresPW(),
		util.GameServerEnvironment.GetPostgresDB(),
	)
}
