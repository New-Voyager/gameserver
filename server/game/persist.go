package game

type PersistHandState interface {
	Load(gameCode string) (*HandState, error)
	Save(gameCode string, state *HandState) error
	Remove(gameCode string) error
}
