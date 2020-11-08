package game

type PersistGameState interface {
	Load(clubID uint32, gameID uint64) (*GameState, error)
	Save(clubID uint32, gameID uint64, state *GameState) error
	Remove(clubID uint32, gameID uint64) error
	NextGameId(clubID uint32) (uint64, error)
}

type PersistHandState interface {
	Load(clubID uint32, gameID uint64, handID uint32) (*HandState, error)
	Save(clubID uint32, gameID uint64, handID uint32, state *HandState) error
	Remove(clubID uint32, gameID uint64, handID uint32) error
}
