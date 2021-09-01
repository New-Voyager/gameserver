package internal

import (
	"fmt"

	"voyager.com/server/util"
)

func GetUsersConnStr() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=users sslmode=%s",
		util.Env.GetPostgresHost(),
		util.Env.GetPostgresPort(),
		util.Env.GetPostgresUser(),
		util.Env.GetPostgresPW(),
		util.Env.GetPostgresSSLMode(),
	)
}

func GetGamesConnStr() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=livegames sslmode=%s",
		util.Env.GetPostgresHost(),
		util.Env.GetPostgresPort(),
		util.Env.GetPostgresUser(),
		util.Env.GetPostgresPW(),
		util.Env.GetPostgresSSLMode(),
	)
}
