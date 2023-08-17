module github.com/distribution/distribution/v3

go 1.18

require (
	cloud.google.com/go/storage v1.30.1
	github.com/AdaLogics/go-fuzz-headers v0.0.0-20221103172237-443f56ff4ba8
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.6.0
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.3.0
	github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.0.0
	github.com/Shopify/logrus-bugsnag v0.0.0-20171204204709-577dee27f20d
	github.com/aws/aws-sdk-go v1.43.16
	github.com/bshuster-repo/logrus-logstash-hook v1.0.0
	github.com/bugsnag/bugsnag-go v0.0.0-20141110184014-b1d153021fcd
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c
	github.com/docker/go-metrics v0.0.1
	github.com/docker/libtrust v0.0.0-20150114040149-fa567046d9b1
	github.com/gomodule/redigo v1.8.2
	github.com/gorilla/handlers v1.5.1
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/golang-lru v0.5.4
	github.com/mitchellh/mapstructure v1.5.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.2
	github.com/prometheus/client_golang v1.12.2 // indirect; updated to latest
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.6.1
	github.com/stretchr/testify v1.8.1
	github.com/yvasiyarov/gorelic v0.0.0-20141212073537-a9bba5b9ab50
	go.opentelemetry.io/collector v0.58.0
	go.opentelemetry.io/otel v1.9.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.8.0
	go.opentelemetry.io/otel/sdk v1.9.0
	go.opentelemetry.io/otel/trace v1.9.0
	golang.org/x/crypto v0.7.0
	golang.org/x/oauth2 v0.6.0
	google.golang.org/api v0.114.0
	google.golang.org/grpc v1.53.0
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
	gopkg.in/yaml.v2 v2.4.0
)

require (
	cloud.google.com/go v0.110.0 // indirect
	contrib.go.opencensus.io/exporter/prometheus v0.4.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.3.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bitly/go-simplejson v0.5.0 // indirect
	github.com/bugsnag/osext v0.0.0-20130617224835-0dd3f918b21b // indirect
	github.com/bugsnag/panicwrap v0.0.0-20151223152923-e2c28503fcd0 // indirect
	github.com/cenkalti/backoff/v4 v4.1.3 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/go-kit/log v0.2.0 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/googleapis/gax-go/v2 v2.7.1 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.7.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.15.9
	github.com/knadh/koanf v1.4.2 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.6 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/osext v0.0.0-20151018003038-5e2d6d41470f // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mostynb/go-grpc-compression v1.1.17 // indirect
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/prometheus/statsd_exporter v0.21.0 // indirect
	github.com/rogpeppe/go-internal v1.6.1 // indirect
	github.com/rs/cors v1.8.2 // indirect
	github.com/shirou/gopsutil/v3 v3.22.7 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tklauser/go-sysconf v0.3.10 // indirect
	github.com/tklauser/numcpus v0.4.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	github.com/yvasiyarov/go-metrics v0.0.0-20140926110328-57bccd1ccd43 // indirect
	github.com/yvasiyarov/newrelic_platform_go v0.0.0-20140908184405-b21fdbd4370f // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/collector/pdata v0.56.0 // indirect
	go.opentelemetry.io/collector/semconv v0.56.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.34.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.34.0 // indirect
	go.opentelemetry.io/contrib/zpages v0.34.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.8.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.8.0 // indirect
	go.opentelemetry.io/otel/exporters/prometheus v0.31.0 // indirect
	go.opentelemetry.io/otel/metric v0.31.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.31.0 // indirect
	go.opentelemetry.io/proto/otlp v0.18.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.21.0 // indirect
	golang.org/x/net v0.8.0 // indirect; updated for CVE-2022-27664, CVE-2022-41717
	golang.org/x/sys v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230320184635-7606e756e683 // indirect
	google.golang.org/protobuf v1.29.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

require (
	cloud.google.com/go/compute v1.18.0 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	cloud.google.com/go/iam v0.12.0 // indirect
	github.com/cyphar/filepath-securejoin v0.2.3 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.3 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
)
