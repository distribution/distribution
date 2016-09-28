package main

import (
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"gopkg.in/yaml.v2"

	log "github.com/Sirupsen/logrus"

	// Register the DTR authorizer.
	"github.com/docker/dhe-deploy"
	_ "github.com/docker/dhe-deploy/garant/authz"
	"github.com/docker/dhe-deploy/hubconfig"
	"github.com/docker/dhe-deploy/hubconfig/etcd"
	"github.com/docker/dhe-deploy/hubconfig/util"
	"github.com/docker/dhe-deploy/manager/schema"
	"github.com/docker/dhe-deploy/registry/middleware"
	"github.com/docker/dhe-deploy/shared/containers"
	"github.com/docker/dhe-deploy/shared/dtrutil"

	// register all storage and auth drivers
	_ "github.com/docker/distribution/registry/auth/htpasswd"
	_ "github.com/docker/distribution/registry/auth/silly"
	_ "github.com/docker/distribution/registry/auth/token"
	_ "github.com/docker/distribution/registry/proxy"
	_ "github.com/docker/distribution/registry/storage/driver/azure"
	_ "github.com/docker/distribution/registry/storage/driver/filesystem"
	_ "github.com/docker/distribution/registry/storage/driver/gcs"
	_ "github.com/docker/distribution/registry/storage/driver/inmemory"
	_ "github.com/docker/distribution/registry/storage/driver/middleware/cloudfront"
	_ "github.com/docker/distribution/registry/storage/driver/oss"
	_ "github.com/docker/distribution/registry/storage/driver/s3-aws"
	_ "github.com/docker/distribution/registry/storage/driver/swift"

	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry"
	"github.com/docker/distribution/version"
	"github.com/docker/garant"

	// Metadata store
	repomiddleware "github.com/docker/distribution/registry/middleware/repository"
)

const configFilePath = "/config/storage.yml"

func main() {
	log.SetFormatter(new(log.JSONFormatter))
	releaseRestartLock()
	notifyReadOnly()
	setupMiddleware()
	go waitForReload()
	go runGarant()
	runRegistry()
}

func runGarant() {
	log.Info("garant starting")

	app, err := garant.NewApp("/config/garant.yml")
	if err != nil {
		log.Fatalf("unable to initialize token server app: %s", err)
	}

	log.Fatal(app.ListenAndServe())
}

func waitForReload() {
	log.Info("listening for sigusr2")
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR2)
	_ = <-c
	log.Info("got sigusr2! Attempting to shut down safely")

	dtrKVStore := makeKVStore()

	log.Info("getting restart lock")
	// This will block until no other registry is restarting
	err := dtrKVStore.Lock(deploy.RegistryRestartLockPath, []byte(os.Getenv(deploy.ReplicaIDEnvVar)), time.Minute)
	if err != nil {
		log.Fatalf("Failed to get restart lock: %s", err)
	}

	log.Fatal("restarting now")
}

func releaseRestartLock() {
	kvStore := makeKVStore()

	value, err := kvStore.Get(deploy.RegistryRestartLockPath)
	if err != nil {
		log.Infof("No lock found to release: %s", err)
		return
	}
	if string(value) == os.Getenv(deploy.ReplicaIDEnvVar) {
		// Unlock the key so others can restart too
		// TODO: check for intermittent failures and do some retries
		err := kvStore.Delete(deploy.RegistryRestartLockPath)
		log.Infof("removing restart lock: %s", err)
	} else {
		log.Info("someone else is holding the lock, not releasing")
	}
}

func notifyReadOnly() {
	storageFile, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Fatalf("error reading storage.yml: %s", err)
	}
	var storageYML configuration.Configuration
	err = yaml.Unmarshal(storageFile, &storageYML)
	if err != nil {
		log.Fatalf("error unmarshaling storage.yml: %s", err)
	}
	roMode := util.GetReadonlyMode(&storageYML.Storage)
	kvStore := makeKVStore()
	roModePath := path.Join(deploy.RegistryROStatePath, os.Getenv(deploy.ReplicaIDEnvVar))
	if roMode {
		log.Infof("registering self as being in read-only mode at key: %s", roModePath)
		err := kvStore.Put(roModePath, []byte{})
		if err != nil {
			log.Errorf("Failed to register self as read-only: %s", err)
			time.Sleep(1)
			log.Fatalf("Failed to register self as read-only: %s", err)
		}
	} else {
		// TODO: check the type of error and retry if it's an intermittent failure instead of a double delete
		err = kvStore.Delete(roModePath)
		log.Infof("no longer in read-only mode: %s", err)
	}
}

func runRegistry() {
	log.Info("registry starting")

	fp, err := os.Open(configFilePath)
	if err != nil {
		log.Fatalf("unable to open registry config: %s", err)
	}

	defer fp.Close()

	config, err := configuration.Parse(fp)
	if err != nil {
		log.Fatalf("error parsing registry config: %s", err)
	}
	if config.Storage.Type() == "filesystem" {
		params := config.Storage["filesystem"]
		params["rootdirectory"] = "/storage"
		config.Storage["filesystem"] = params
	}

	registry, err := registry.NewRegistry(context.WithVersion(context.Background(), version.Version), config)
	if err != nil {
		log.Fatalf("unable to initialize registry: %s", err)
	}
	log.Fatal(registry.ListenAndServe())
}

// TODO: make don't call this function so many times
func makeKVStore() hubconfig.KeyValueStore {
	dtrKVStore, err := etcd.NewKeyValueStore(containers.EtcdUrls(), deploy.EtcdPath)
	if err != nil {
		log.Fatalf("something went wrong when trying to initialize the Lock: %s", err)
	}
	return dtrKVStore
}

func setupMiddleware() {
	replicaID := os.Getenv(deploy.ReplicaIDEnvVar)
	db, err := dtrutil.GetRethinkSession(replicaID)
	if err != nil {
		log.WithField("error", err).Fatal("failed to connect to rethink")
	}
	store := schema.NewMetadataManager(db)
	middleware.RegisterStore(store)
	if err := repomiddleware.Register("metadata", middleware.InitMiddleware); err != nil {
		log.WithField("err", err).Fatal("unable to register metadata middleware")
	}
	log.Info("connected to middleware")
}
