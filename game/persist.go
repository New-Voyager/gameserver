package game

type PersistGameState interface {
	Load(clubID uint32, gameID uint32) (*GameState, error)
	Save(clubID uint32, gameID uint32, state *GameState) error
	Remove(clubID uint32, gameID uint32) error
	NextGameId(clubID uint32) (uint32, error)
}

type PersistHandState interface {
	Load(clubID uint32, gameID uint32, handID uint32) (*HandState, error)
	Save(clubID uint32, gameID uint32, handID uint32, state *HandState) error
	Remove(clubID uint32, gameID uint32, handID uint32) error
}
