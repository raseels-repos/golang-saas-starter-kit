package devops

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
)

// SyncCfgInit provides the functionality to keep config files sync'd between running tasks and across deployments.
func SyncCfgInit(log *log.Logger, awsSession *session.Session, secretPrefix, watchDir string, syncInterval time.Duration) (func(), error) {

	localfiles := make(map[string]time.Time)

	// Do the initial sync before starting file watch to download any existing configs.
	err :=  SyncCfgDir(log, awsSession, secretPrefix, watchDir, localfiles)
	if err != nil {
		return nil, err
	}

	// Create a new file watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Return function that will should be run in the back ground via a go routine that will watch for new files created
	// locally and updated in AWS Secrets Manager.
	f := func() {
		defer watcher.Close()

		// Init the watch to wait for sync local files to Secret Manager.
		WatchCfgDir(log, awsSession, secretPrefix, watchDir, watcher, localfiles)


		// Init ticker to sync remote files from Secret Manager locally at the defined interval.
		if syncInterval.Seconds() > 0 {
			ticker := time.NewTicker(syncInterval)
			defer ticker.Stop()

			go func() {
				for _ = range ticker.C {
					log.Println("AWS Secrets Manager : Checking for remote updates")

					// Do the initial sync before starting file watch to download any existing configs.
					err :=  SyncCfgDir(log, awsSession, secretPrefix, watchDir, localfiles)
					if err != nil {
						log.Printf("AWS Secrets Manager : Remote sync error - %+v", err)
					}
				}
			}()
		}
	}

	log.Printf("AWS Secrets Manager : Watching config dir %s", watchDir)

	// Note: Out of the box fsnotify can watch a single file, or a single directory.
	if err := watcher.Add(watchDir); err != nil {
		return nil, errors.Wrapf(err, "failed to add file watcher to %s", watchDir)
	}

	return f, nil
}

// SyncCfgDir lists all the Secrets from AWS Secrets Manager for a provided prefix and downloads them locally.
func SyncCfgDir(log *log.Logger, awsSession *session.Session, secretPrefix, watchDir string, localfiles map[string]time.Time) error {

	svc := secretsmanager.New(awsSession)

	// Get a list of secrets for the prefix when the time they were last changed.
	secretIDs := make(map[string]time.Time)
	err := svc.ListSecretsPages(&secretsmanager.ListSecretsInput{}, func(res *secretsmanager.ListSecretsOutput, lastPage bool) bool {
		for _, s := range res.SecretList {

			// Skip any secret that does not have a matching prefix.
			if !strings.HasPrefix(*s.Name, secretPrefix)  {
				continue
			}

			secretIDs[*s.Name] = s.LastChangedDate.UTC()
		}

		return !lastPage
	})

	if err != nil {
		return errors.Wrap(err, "failed to list secrets")
	}

	for id, curChanged := range secretIDs {

		// Load the secret by ID from Secrets Manager.
		res, err := svc.GetSecretValue(&secretsmanager.GetSecretValueInput{
			SecretId: aws.String(id),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to get secret value for id %s", id)
		}

		filename := filepath.Base(id)
		localpath := filepath.Join(watchDir, filename)

		// Ensure the secret exists locally.
		if exists(localpath) {
			// If the secret was previously downloaded and current last changed time is less than or equal to the time
			// the secret was last downloaded, then no need to update.
			if lastChanged, ok := localfiles[id]; ok && curChanged.UTC().Unix() <= lastChanged.UTC().Unix() {
				continue
			}

		}

		log.Printf("AWS Secrets Manager : Writing Config %s", filename)
		err = ioutil.WriteFile(localpath, res.SecretBinary, 0644)
		if err != nil {
			return errors.Wrapf(err, "failed to write secret value for id %s to %s", id, localpath)
		}

		// Only mark that the secret was updated when the file was successfully saved locally.
		localfiles[id] = curChanged
	}

	return nil
}

// WatchCfgDir watches for new/updated files locally and uploads them to in AWS Secrets Manager.
func WatchCfgDir(log *log.Logger, awsSession *session.Session, secretPrefix, dir string, watcher *fsnotify.Watcher, localfiles map[string]time.Time) error {

	for {
		select {
		// watch for events
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			err := handleWatchCfgEvent(log, awsSession, secretPrefix, event)
			if err != nil {
				log.Printf("AWS Secrets Manager : Watcher Error - %+v", err)
			}

		// watch for errors
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			if err != nil {
				log.Printf("AWS Secrets Manager : Watcher Error - %+v", err)
			}
		}
	}

	return nil
}

// handleWatchCfgEvent handles a fsnotify event. For new files, secrets are created, for updated files, the secret is
// updated. For deleted files the secret is removed.
func handleWatchCfgEvent(log *log.Logger, awsSession *session.Session, secretPrefix string, event fsnotify.Event) error {

	svc := secretsmanager.New(awsSession)

	fname := filepath.Base(event.Name)
	secretID := filepath.Join(secretPrefix, fname)

	if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {

		dat, err := ioutil.ReadFile(event.Name)
		if err != nil {
			return errors.Wrapf(err, "file watcher failed to read file %s", event.Name)
		}

		// Create the new entry in AWS Secret Manager for the file.
		_, err = svc.CreateSecret(&secretsmanager.CreateSecretInput{
			Name:         aws.String(secretID),
			SecretString: aws.String(string(dat)),
		})
		if err != nil {
			if aerr, ok := err.(awserr.Error); !ok {

				if aerr.Code() == secretsmanager.ErrCodeInvalidRequestException {
					// InvalidRequestException: You can't create this secret because a secret with this
					// 							 name is already scheduled for deletion.

					// Restore secret after it was already previously deleted.
					_, err = svc.RestoreSecret(&secretsmanager.RestoreSecretInput{
						SecretId:         aws.String(secretID),
					})
					if err != nil {
						return errors.Wrapf(err, "file watcher failed to restore secret %s for %s", secretID, event.Name)
					}

				} else if aerr.Code() != secretsmanager.ErrCodeResourceExistsException {
					return errors.Wrapf(err, "file watcher failed to create secret %s for %s", secretID, event.Name)
				}
			}

			// If where was a resource exists error for create, then need to update the secret instead.
			_, err = svc.UpdateSecret(&secretsmanager.UpdateSecretInput{
				SecretId:         aws.String(secretID),
				SecretString: aws.String(string(dat)),
			})
			if err != nil {
				return errors.Wrapf(err, "file watcher failed to update secret %s for %s", secretID, event.Name)
			}

			log.Printf("AWS Secrets Manager : Secret %s updated for %s", secretID, event.Name)
		} else {
			log.Printf("AWS Secrets Manager : Secret %s created for %s", secretID, event.Name)
		}

	} else if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
		// Delay delete to ensure the file is really deleted.
		//delCheck := time.NewTimer(time.Minute)

		//<-delCheck.C

		// Create the new entry in AWS Secret Manager for the file.
		_, err := svc.DeleteSecret(&secretsmanager.DeleteSecretInput{
			SecretId:         aws.String(secretID),

			// (Optional) Specifies that the secret is to be deleted without any recovery
			// window. You can't use both this parameter and the RecoveryWindowInDays parameter
			// in the same API call.
			//
			// An asynchronous background process performs the actual deletion, so there
			// can be a short delay before the operation completes. If you write code to
			// delete and then immediately recreate a secret with the same name, ensure
			// that your code includes appropriate back off and retry logic.
			//
			// Use this parameter with caution. This parameter causes the operation to skip
			// the normal waiting period before the permanent deletion that AWS would normally
			// impose with the RecoveryWindowInDays parameter. If you delete a secret with
			// the ForceDeleteWithouRecovery parameter, then you have no opportunity to
			// recover the secret. It is permanently lost.
			ForceDeleteWithoutRecovery: aws.Bool(false),

			// (Optional) Specifies the number of days that Secrets Manager waits before
			// it can delete the secret. You can't use both this parameter and the ForceDeleteWithoutRecovery
			// parameter in the same API call.
			//
			// This value can range from 7 to 30 days.
			RecoveryWindowInDays: aws.Int64(30),
		})
		if err != nil {
			return errors.Wrapf(err, "file watcher failed to delete secret %s for %s", secretID, event.Name)
		}

		log.Printf("AWS Secrets Manager : Secret %s deleted for %s", secretID, event.Name)
	}

	return nil
}

// Exists reports whether the named file or directory exists.
func exists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}
