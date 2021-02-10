package util

// GetDriverToGameSubject returns the nats subject used for driver -> game communication.
func GetDriverToGameSubject() string {
	return "driverbot.game"
}
