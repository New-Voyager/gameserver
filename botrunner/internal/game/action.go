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
