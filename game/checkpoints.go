package game

const (
	CheckPoint__START1      uint32 = 1
	CheckPoint__START2      uint32 = 2
	CheckPoint__START3      uint32 = 3
	CheckPoint__RESULT_SENT uint32 = 11
	CheckPoint__DEAL1       uint32 = 21
	CheckPoint__DEAL2       uint32 = 22
	CheckPoint__DEAL3       uint32 = 23
	CheckPoint__DEAL4       uint32 = 24

	GameStage__NEW_HAND                 uint32 = 1
	GameStage__WAIT_FOR_NEXT_ACTION     uint32 = 2
	GameStage__PREPARE_NEXT_ACTION      uint32 = 3
	GameStage__MOVE_TO_NEXT_ACTION      uint32 = 4
	GameStage__NEXT_ROUND               uint32 = 5
	GameStage__RESULT                   uint32 = 6
	GameStage__HAND_END                 uint32 = 7
	GameStage__HEADS_UP_PROMPT          uint32 = 8
	GameStage__HEADS_UP_PROMPT_RESPONSE uint32 = 9
)
