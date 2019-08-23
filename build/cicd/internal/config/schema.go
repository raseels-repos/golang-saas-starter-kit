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

	cfgCtx, err := NewConfigContext(targetEnv, awsCredentials)
	if err != nil {
		return err
	}

	cfg, err := cfgCtx.Config(log)
	if err != nil {
		return err
	}

	err = devdeploy.SetupDeploymentEnv(log, cfg)
	if err != nil {
		return err
	}

	masterDb, err := sqlx.Open(cfg.DBConnInfo.Driver, cfg.DBConnInfo.URL())
	if err != nil {
		return errors.Wrap(err, "Failed to connect to db for schema migration.")
	}
	defer masterDb.Close()

	return schema.Migrate(context.Background(), targetEnv, masterDb, log, false)
}
