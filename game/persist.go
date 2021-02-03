package game

type PersistGameState interface {
	Load(gameCode string) (*GameState, error)
	Save(gameCode string, state *GameState) error
	Remove(gameCode string) error
}

type PersistHandState interface {
	Load(gameCode string, handID uint32) (*HandState, error)
	LoadClone(gameCode string, handID uint32) (*HandState, error)
	Save(gameCode string, handID uint32, state *HandState) error
	SaveClone(gameCode string, handID uint32, state *HandState) error
	Remove(gameCode string, handID uint32) error
	RemoveClone(gameCode string, handID uint32) error
}

type PersistGameUpdatesState interface {
	Load(gameCode string) (*PendingGameUpdates, error)
	Save(gameCode string, state *PendingGameUpdates) error
	Remove(gameCode string) error
}
