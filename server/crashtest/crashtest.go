package crashtest

import (
	"fmt"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"voyager.com/logging"
	"voyager.com/server/internal"
	"voyager.com/server/util"
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
	CrashPoint_PENDING_UPDATES_1      CrashPoint = "PENDING_UPDATES_1"
	CrashPoint_PENDING_UPDATES_2      CrashPoint = "PENDING_UPDATES_2"
	CrashPoint_PENDING_UPDATES_3      CrashPoint = "PENDING_UPDATES_3"
)

var (
	CrashSetLock    sync.Mutex
	exitProcessFunc func()
)

func SetExitFunc(exitFunc func()) {
	exitProcessFunc = exitFunc
}

// IsValid checks if cp is a valid enum value for CrashPoint.
func (cp CrashPoint) IsValid() bool {
	switch cp {
	case CrashPoint_NO_CRASH, CrashPoint_NOW, CrashPoint_DEAL_1, CrashPoint_DEAL_2, CrashPoint_DEAL_3, CrashPoint_DEAL_4, CrashPoint_DEAL_5, CrashPoint_DEAL_6, CrashPoint_WAIT_FOR_NEXT_ACTION_1, CrashPoint_WAIT_FOR_NEXT_ACTION_2, CrashPoint_PREPARE_NEXT_ACTION_1, CrashPoint_PREPARE_NEXT_ACTION_2, CrashPoint_PREPARE_NEXT_ACTION_3, CrashPoint_PENDING_UPDATES_1, CrashPoint_PENDING_UPDATES_2, CrashPoint_PENDING_UPDATES_3:
		return true
	}
	return false
}

var (
	crashGameCode string
	crashPoint    CrashPoint = CrashPoint_NO_CRASH
	crashPlayerID uint64

	crashTestLogger = logging.GetZeroLogger("crashtest::crashtest", nil)
)

// Set schedules for crashing at the specified point.
// If cp == CrashPoint_NOW, the function will crash immediately without returning.
func Set(gameCode string, cp CrashPoint, playerID uint64) error {
	if !util.Env.IsSystemTest() {
		fmt.Println("crashtest.Set called when not in system test.")
		return nil
	}

	CrashSetLock.Lock()
	defer CrashSetLock.Unlock()

	if crashPoint != CrashPoint_NO_CRASH {
		return fmt.Errorf("Cannot set crashpoint [%s] when previous crash point [%s/%d/%s] hasn't been hit", cp, crashGameCode, crashPlayerID, crashPoint)
	}

	if !cp.IsValid() {
		return fmt.Errorf("Invalid crash point enum: [%s]", cp)
	}

	if cp == CrashPoint_NOW {
		fmt.Printf("CRASHTEST Set called with NOW. Exiting immediately.")
		exitProcessFunc()
	}

	crashGameCode = gameCode
	crashPoint = cp
	crashPlayerID = playerID
	return nil
}

// Hit will crash the process if cp matches the crash point scheduled by Set.
func Hit(gameCode string, cp CrashPoint, playerID uint64) {
	CrashSetLock.Lock()
	defer CrashSetLock.Unlock()

	if cp != crashPoint || playerID != crashPlayerID {
		return
	}

	if !util.Env.IsSystemTest() {
		fmt.Println("Ignoring crashtest.Hit since we are not in system test.")
		return
	}

	// Save to the crash tracking trable.
	err := saveToTracker(gameCode, cp)
	if err != nil {
		fmt.Printf("Error while inserting crash record. Game Code: %s, Crash Point: %s, Error: %s\n", gameCode, cp, err)
	} else {
		fmt.Printf("CRASHTEST GameCode: %s CrashPoint: %s, CrashPlayerID: %d\n", gameCode, cp, crashPlayerID)
	}

	// exitProcessFunc may not exit the process immediately.
	// Add some sleep after to block the calling code from progressing any further
	// before the process exits.
	exitProcessFunc()
	time.Sleep(10 * time.Second)
}

func saveToTracker(gameCode string, cp CrashPoint) error {
	db := sqlx.MustConnect("postgres", internal.GetCrashDBConnStr())
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
