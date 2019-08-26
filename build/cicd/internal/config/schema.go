package config

import (
	"context"
	"log"

	"geeks-accelerator/oss/saas-starter-kit/internal/schema"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"gitlab.com/geeks-accelerator/oss/devops/pkg/devdeploy"
)

// RunSchemaMigrationsForTargetEnv executes schema migrations for the target environment.
func RunSchemaMigrationsForTargetEnv(log *log.Logger, awsCredentials devdeploy.AwsCredentials, targetEnv Env, isUnittest bool) error {

	cfg, err := NewConfig(log, targetEnv, awsCredentials)
	if err != nil {
		return err
	}

	infra, err := devdeploy.SetupInfrastructure(log, cfg)
	if err != nil {
		return err
	}

	connInfo, err := cfg.GetDBConnInfo(infra)
	if err != nil {
		return err
	}

	masterDb, err := sqlx.Open(connInfo.Driver, connInfo.URL())
	if err != nil {
		return errors.Wrap(err, "Failed to connect to db for schema migration.")
	}
	defer masterDb.Close()

	return schema.Migrate(context.Background(), targetEnv, masterDb, log, false)
}
