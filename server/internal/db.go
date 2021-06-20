package internal

import (
	"fmt"

	"voyager.com/server/util"
)

func GetConnStr() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		util.GameServerEnvironment.GetPostgresHost(),
		util.GameServerEnvironment.GetPostgresPort(),
		util.GameServerEnvironment.GetPostgresUser(),
		util.GameServerEnvironment.GetPostgresPW(),
		util.GameServerEnvironment.GetPostgresDB(),
	)
}
