package internal

import (
	"fmt"

	"voyager.com/server/util"
)

func GetUsersConnStr() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=users sslmode=disable",
		util.GameServerEnvironment.GetPostgresHost(),
		util.GameServerEnvironment.GetPostgresPort(),
		util.GameServerEnvironment.GetPostgresUser(),
		util.GameServerEnvironment.GetPostgresPW(),
	)
}

func GetGamesConnStr() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=livegames sslmode=disable",
		util.GameServerEnvironment.GetPostgresHost(),
		util.GameServerEnvironment.GetPostgresPort(),
		util.GameServerEnvironment.GetPostgresUser(),
		util.GameServerEnvironment.GetPostgresPW(),
	)
}
