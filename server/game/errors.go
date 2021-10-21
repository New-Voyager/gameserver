package game

import "fmt"

type UnexpectedGameStatusError struct {
	GameStatus  GameStatus
	TableStatus TableStatus
}

func (e UnexpectedGameStatusError) Error() string {
	return fmt.Sprintf("Unexpected game status and table status: %s/%s", e.GameStatus, e.TableStatus)
}

type NotReadyToDealError struct {
	Msg string
}

func (e NotReadyToDealError) Error() string {
	return e.Msg
}
