package game

const (
	GameStage__NEW_HAND                 uint32 = 1
	GameStage__WAIT_FOR_NEXT_ACTION     uint32 = 2
	GameStage__PREPARE_NEXT_ACTION      uint32 = 3
	GameStage__MOVE_TO_NEXT_ACTION      uint32 = 4
	GameStage__MOVE_TO_NEXT_ROUND       uint32 = 5
	GameStage__ALL_PLAYERS_ALL_IN       uint32 = 6
	GameStage__ONE_PLAYER_REMAINING     uint32 = 7
	GameStage__SHOWDOWN                 uint32 = 8
	GameStage__HAND_ENDED               uint32 = 9
	GameStage__HEADS_UP_PROMPT          uint32 = 10
	GameStage__HEADS_UP_PROMPT_RESPONSE uint32 = 11
)
