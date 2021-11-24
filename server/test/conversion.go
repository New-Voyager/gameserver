package test

import (
	"fmt"

	"voyager.com/server/game"
)

func ToGameChipUnit(str string) game.ChipUnit {
	if str == "DOLLAR" {
		return game.ChipUnit_DOLLAR
	}
	if str == "" || str == "CENT" {
		return game.ChipUnit_CENT
	}
	panic(fmt.Sprintf("Invalid chip unit: %s", str))
}
