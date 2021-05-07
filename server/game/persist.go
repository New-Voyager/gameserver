package game

type PersistHandState interface {
	Load(gameCode string) (*HandState, error)
	LoadClone(gameCode string) (*HandState, error)
	Save(gameCode string, state *HandState) error
	SaveClone(gameCode string, state *HandState) error
	Remove(gameCode string) error
	RemoveClone(gameCode string) error
}
