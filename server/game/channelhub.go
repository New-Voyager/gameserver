package game

// GameHub ties game and players, which use
// go channels for the communication.
// This hub is used for unit testing and game script testing
// without any infrastructure.
// All inbound and outboud messages in ChannelHub are in protobuf format.
// Game and players deal with channel hub.
// GameHub runs a go routine to get messages that are destined to game or hand
// Game also sends messages via channels to the hub
// GameHub is responsible tracking slow connections, network connection issues
// players left the game etc.,
type GameHub struct {
	// game object
	game *Game

	// players who is either in the table or observing
	players map[uint32]*Player
}

func NewGameHub() *GameHub {
	return &GameHub{}
}

func (c *GameHub) InitializeGame(clubID uint32, gameID uint32) error {
	// TODO: We should fetch the game configuration from API server
	return nil
}

func (c *GameHub) InitializeGameFromConfig(clubID uint32, gameID uint32, gameConfig *GameConfig) error {
	// TODO: We should fetch the game configuration from API server
	return nil
}

func (c *GameHub) StartGame(clubID uint32, gameID uint32) error {
	return nil
}

// SendMessageToGame: a player sends message to game channel
func (c *GameHub) PrivateMessageToGame(playerID uint32, message *GameMessage) error {
	return nil
}

// SendMessageToHand: a player sends a message to hand channel
func (c *GameHub) PrivateMessageToHand(playerID uint32, message *HandMessage) error {
	return nil
}

func (c *GameHub) GameToPlayer(playerID uint32, message *GameMessage) error {
	return nil
}

func (c *GameHub) BroadcastHand(message *GameMessage) error {
	return nil
}

func (c *GameHub) HandToPlayer(playerID uint32, message *HandMessage) error {
	return nil
}

func (c *GameHub) HandToAllPlayers(playerID uint32, message *HandMessage) error {
	return nil
}
