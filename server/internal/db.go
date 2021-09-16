package internal

import (
	"fmt"

	"voyager.com/server/util"
)

func GetCrashDBConnStr() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		util.Env.GetPostgresHost(),
		util.Env.GetPostgresPort(),
		util.Env.GetPostgresUser(),
		util.Env.GetPostgresPW(),
		util.Env.GetPostgresCrashDB(),
		util.Env.GetPostgresSSLMode(),
	)
}
