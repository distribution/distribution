module github.com/distribution/distribution/v3

go 1.16

require (
	github.com/Azure/azure-sdk-for-go v56.3.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.24 // indirect
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Shopify/logrus-bugsnag v0.0.0-20171204204709-577dee27f20d
	github.com/aws/aws-sdk-go v1.43.16
	github.com/bitly/go-simplejson v0.5.0 // indirect
	github.com/bshuster-repo/logrus-logstash-hook v1.0.0
	github.com/bugsnag/bugsnag-go v0.0.0-20141110184014-b1d153021fcd
	github.com/bugsnag/osext v0.0.0-20130617224835-0dd3f918b21b // indirect
	github.com/bugsnag/panicwrap v0.0.0-20151223152923-e2c28503fcd0 // indirect
	github.com/denverdino/aliyungo v0.0.0-20190125010748-a747050bb1ba
	github.com/dnaeon/go-vcr v1.0.1 // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c
	github.com/docker/go-metrics v0.0.1
	github.com/docker/libtrust v0.0.0-20150114040149-fa567046d9b1
	github.com/gofrs/uuid v4.0.0+incompatible // indirect
	github.com/gomodule/redigo v1.8.2
	github.com/gorilla/handlers v1.5.1
	github.com/gorilla/mux v1.8.0
	github.com/mitchellh/mapstructure v1.1.2
	github.com/mitchellh/osext v0.0.0-20151018003038-5e2d6d41470f // indirect
	github.com/ncw/swift v1.0.47
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.2
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.0.0
	github.com/yvasiyarov/go-metrics v0.0.0-20140926110328-57bccd1ccd43 // indirect
	github.com/yvasiyarov/gorelic v0.0.0-20141212073537-a9bba5b9ab50
	github.com/yvasiyarov/newrelic_platform_go v0.0.0-20140908184405-b21fdbd4370f // indirect
	github.com/zeebo/errs v1.3.0
	golang.org/x/crypto v0.0.0-20220131195533-30dcbda58838
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	google.golang.org/api v0.1.0
	// when updating google.golang.org/cloud, update (or remove) the replace
	// rule for google.golang.org/grpc accordingly.
	google.golang.org/cloud v0.0.0-20151119220103-975617b05ea8
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127
	gopkg.in/yaml.v2 v2.4.0
	storj.io/uplink v1.8.2-0.20220426114611-5580401cd102
)

// Prevent unwanted updates of grpc. In our codebase, it's a dependency of
// google.golang.org/cloud. However, github.com/spf13/viper (which is an indirect
// dependency of github.com/spf13/cobra) declares a more recent version. Viper
// is not used in the codebase, but go modules uses the go.mod of *all* dependen-
// cies to determine the minimum version of a module, but does *not* check if that
// depdendency's code using the dependency is actually used.
//
// In our case, github.com/spf13/viper occurs as a dependency, but is unused,
// so we can ignore the minimum versions of grpc and jwt-go that it specifies.
replace google.golang.org/grpc => google.golang.org/grpc v0.0.0-20160317175043-d3ddb4469d5a
