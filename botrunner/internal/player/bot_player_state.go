package player

const (
	BotState__UNKNOWN               string = "UNKNOWN"
	BotState__NOT_IN_GAME           string = "NOT_IN_GAME"
	BotState__OBSERVING             string = "OBSERVING"
	BotState__JOINING               string = "JOINING"
	BotState__WAITING_FOR_MY_TURN   string = "WAITING_FOR_MY_TURN"
	BotState__MY_TURN               string = "MY_TURN"
	BotState__ACTED_WAITING_FOR_ACK string = "ACTED_WAITING_FOR_ACK"
	BotState__REJOINING             string = "REJOINING"
	BotState__ERROR                 string = "ERROR"

	BotEvent__SUBSCRIBE           string = "SUBSCRIBE"
	BotEvent__REQUEST_SIT         string = "REQUEST_SIT"
	BotEvent__SUCCEED_BUYIN       string = "SUCCEED_BUYIN"
	BotEvent__REJOIN              string = "REJOIN"
	BotEvent__RECEIVE_YOUR_ACTION string = "RECEIVE_YOUR_ACTION"
	BotEvent__SEND_MY_ACTION      string = "SEND_MY_ACTION"
	BotEvent__RECEIVE_ACK         string = "RECEIVE_ACK"
	BotEvent__ACTION_TIMEDOUT     string = "ACTION_TIMEDOUT"
)
