package crashtest

import (
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
)

// CrashPoint is an enum representing different points in the code that the server can crash.
type CrashPoint string

const (
	CrashPoint_NO_CRASH             CrashPoint = "NO_CRASH"
	CrashPoint_NOW                  CrashPoint = "NOW"
	CrashPoint_WAIT_FOR_NEXT_ACTION CrashPoint = "WAIT_FOR_NEXT_ACTION"
	CrashPoint_PREPARE_NEXT_ACTION  CrashPoint = "PREPARE_NEXT_ACTION"
)

// IsValid checks if cp is a valid enum value for CrashPoint.
func (cp CrashPoint) IsValid() error {
	switch cp {
	case CrashPoint_NO_CRASH, CrashPoint_NOW, CrashPoint_WAIT_FOR_NEXT_ACTION, CrashPoint_PREPARE_NEXT_ACTION:
		return nil
	}
	return fmt.Errorf("Invalid crash point [%s]", cp)
}

var crashTestLogger = log.With().Str("logger_name", "crashtest::controller").Logger()
var crashAt CrashPoint = CrashPoint_NO_CRASH

// Set schedules for crashing at the specified point.
// If cp == CrashPoint_NOW, the function will crash immediately without returning.
func Set(cp CrashPoint) error {
	crashAt = cp
	if cp == CrashPoint_NOW {
		Hit(CrashPoint_NOW)
	}
	return nil
}

// Hit will panic and crash the process if cp matches the crash point scheduled by Set.
func Hit(cp CrashPoint) {
	if cp == crashAt {
		fmt.Printf("CRASHTEST: %s\n", cp)
		os.Exit(1)
	}
}
