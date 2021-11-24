package game

import "voyager.com/server/util"

type ChipConverter struct {
	ChipUnit ChipUnit
}

func NewChipConverter(chipUnit ChipUnit) *ChipConverter {
	return &ChipConverter{ChipUnit: chipUnit}
}

func (c *ChipConverter) ChipsToCents(chips float64) float64 {
	return util.ChipsToCents(int32(c.ChipUnit), chips)
}

func (c *ChipConverter) CentsToChips(cents float64) float64 {
	return util.CentsToChips(int32(c.ChipUnit), cents)
}
