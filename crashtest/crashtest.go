package crashtest

import (
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
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

	ExitCode int = 66
)

// IsValid checks if cp is a valid enum value for CrashPoint.
func (cp CrashPoint) IsValid() bool {
	switch cp {
	case CrashPoint_NO_CRASH, CrashPoint_NOW, CrashPoint_WAIT_FOR_NEXT_ACTION_1, CrashPoint_WAIT_FOR_NEXT_ACTION_2, CrashPoint_PREPARE_NEXT_ACTION_1, CrashPoint_PREPARE_NEXT_ACTION_2, CrashPoint_PREPARE_NEXT_ACTION_3, CrashPoint_PREPARE_NEXT_ACTION_4:
		return true
	}
	return false
}

var crashTestLogger = log.With().Str("logger_name", "crashtest::crashtest").Logger()
var crashAt CrashPoint = CrashPoint_NO_CRASH

// Set schedules for crashing at the specified point.
// If cp == CrashPoint_NOW, the function will crash immediately without returning.
func Set(cp CrashPoint) error {
	if !cp.IsValid() {
		return fmt.Errorf("Invalid crash point enum: [%s]", cp)
	}
	crashAt = cp
	if cp == CrashPoint_NOW {
		Hit(CrashPoint_NOW)
	}
	return nil
}

// Hit will crash the process if cp matches the crash point scheduled by Set.
func Hit(cp CrashPoint) {
	if cp == crashAt {
		fmt.Printf("CRASHTEST: %s\n", cp)
		os.Exit(ExitCode)
	}
}
