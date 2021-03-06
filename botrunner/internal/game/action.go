package game

// ActionContains checks if the slice of actions contains the action.
func ActionContains(actions []ACTION, action ACTION) bool {
	for _, a := range actions {
		if a == action {
			return true
		}
	}
	return false
}

// ActionStringToAction converts string to action enum.
// "BET" => ACTION_BET
func ActionStringToAction(actionStr string) ACTION {
	return ACTION(ACTION_value[actionStr])
}
