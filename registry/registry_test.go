package registry

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/docker/distribution/configuration"
	dcontext "github.com/docker/distribution/context"
	_ "github.com/docker/distribution/registry/storage/driver/inmemory"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// Tests to ensure nextProtos returns the correct protocols when:
// * config.HTTP.HTTP2.Disabled is not explicitly set => [h2 http/1.1]
// * config.HTTP.HTTP2.Disabled is explicitly set to false [h2 http/1.1]
// * config.HTTP.HTTP2.Disabled is explicitly set to true [http/1.1]
func TestNextProtos(t *testing.T) {
	config := &configuration.Configuration{}
	protos := nextProtos(config)
	if !reflect.DeepEqual(protos, []string{"h2", "http/1.1"}) {
		t.Fatalf("expected protos to equal [h2 http/1.1], got %s", protos)
	}
	config.HTTP.HTTP2.Disabled = false
	protos = nextProtos(config)
	if !reflect.DeepEqual(protos, []string{"h2", "http/1.1"}) {
		t.Fatalf("expected protos to equal [h2 http/1.1], got %s", protos)
	}
	config.HTTP.HTTP2.Disabled = true
	protos = nextProtos(config)
	if !reflect.DeepEqual(protos, []string{"http/1.1"}) {
		t.Fatalf("expected protos to equal [http/1.1], got %s", protos)
	}
}

func setupRegistry() (*Registry, error) {
	config := &configuration.Configuration{}
	// TODO: this needs to change to something ephemeral as the test will fail if there is any server
	// already listening on port 5000
	config.HTTP.Addr = ":5000"
	config.HTTP.DrainTimeout = time.Duration(10) * time.Second
	config.Storage = map[string]configuration.Parameters{"inmemory": map[string]interface{}{}}
	return NewRegistry(context.Background(), config)
}

func TestGracefulShutdown(t *testing.T) {
	registry, err := setupRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// run registry server
	var errchan chan error
	go func() {
		errchan <- registry.ListenAndServe()
	}()
	select {
	case err = <-errchan:
		t.Fatalf("Error listening: %v", err)
	default:
	}

	// Wait for some unknown random time for server to start listening
	time.Sleep(3 * time.Second)

	// send incomplete request
	conn, err := net.Dial("tcp", "localhost:5000")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(conn, "GET /v2/ ")

	// send stop signal
	quit <- os.Interrupt
	time.Sleep(100 * time.Millisecond)

	// try connecting again. it shouldn't
	_, err = net.Dial("tcp", "localhost:5000")
	if err == nil {
		t.Fatal("Managed to connect after stopping.")
	}

	// make sure earlier request is not disconnected and response can be received
	fmt.Fprintf(conn, "HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n")
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "200 OK" {
		t.Error("response status is not 200 OK: ", resp.Status)
	}
	if body, err := ioutil.ReadAll(resp.Body); err != nil || string(body) != "{}" {
		t.Error("Body is not {}; ", string(body))
	}
}

func TestConfigureLogging(t *testing.T) {
	yamlConfig := `---
log:
  level: warn
  fields:
    foo: bar
    baz: xyzzy
`

	var config configuration.Configuration
	err := yaml.Unmarshal([]byte(yamlConfig), &config)
	if err != nil {
		t.Fatal("failed to parse config: ", err)
	}

	ctx, err := configureLogging(context.Background(), &config)
	if err != nil {
		t.Fatal("failed to configure logging: ", err)
	}

	// Check that the log level was set to Warn.
	if logrus.IsLevelEnabled(logrus.InfoLevel) {
		t.Error("expected Info to be disabled, is enabled")
	}

	// Check that the returned context's logger includes the right fields.
	logger := dcontext.GetLogger(ctx)
	entry, ok := logger.(*logrus.Entry)
	if !ok {
		t.Fatalf("expected logger to be a *logrus.Entry, is: %T", entry)
	}
	val, ok := entry.Data["foo"].(string)
	if !ok || val != "bar" {
		t.Error("field foo not configured correctly; expected 'bar' got: ", val)
	}
	val, ok = entry.Data["baz"].(string)
	if !ok || val != "xyzzy" {
		t.Error("field baz not configured correctly; expected 'xyzzy' got: ", val)
	}

	// Get a logger for a new, empty context and make sure it also has the right fields.
	logger = dcontext.GetLogger(context.Background())
	entry, ok = logger.(*logrus.Entry)
	if !ok {
		t.Fatalf("expected logger to be a *logrus.Entry, is: %T", entry)
	}
	val, ok = entry.Data["foo"].(string)
	if !ok || val != "bar" {
		t.Error("field foo not configured correctly; expected 'bar' got: ", val)
	}
	val, ok = entry.Data["baz"].(string)
	if !ok || val != "xyzzy" {
		t.Error("field baz not configured correctly; expected 'xyzzy' got: ", val)
	}
}
