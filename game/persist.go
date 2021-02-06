package game

type PersistGameState interface {
	Load(gameCode string) (*GameState, error)
	Save(gameCode string, state *GameState) error
	Remove(gameCode string) error
}

type PersistHandState interface {
	Load(gameCode string) (*HandState, error)
	LoadClone(gameCode string) (*HandState, error)
	Save(gameCode string, state *HandState) error
	SaveClone(gameCode string, state *HandState) error
	Remove(gameCode string) error
	RemoveClone(gameCode string) error
}

type PersistGameUpdatesState interface {
	Load(gameCode string) (*PendingGameUpdates, error)
	Save(gameCode string, state *PendingGameUpdates) error
	Remove(gameCode string) error
}
