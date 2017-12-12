package newrelic_platform_go

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Mocks a Metrica object
type MockMetrica struct {
	Value float64
	Name  string
	Units string
}

// Gets a default new mock metrica
func NewMockMetrica() MockMetrica {
	return MockMetrica{
		Value: 5.2,
		Name:  "mock-metrica",
		Units: "flots",
	}
}

func (m MockMetrica) GetValue() (float64, error) {
	return m.Value, nil
}
func (m MockMetrica) GetName() string {
	return m.Name
}
func (m MockMetrica) GetUnits() string {
	return m.Units
}

// Struct representing a mock of New Relic
type MockNewRelic struct {
	RequestFunc http.HandlerFunc
	Server      *httptest.Server
}

// Create a new mock for the New Relic service
func NewMockNewRelic() MockNewRelic {
	mock := MockNewRelic{}
	mock.RequestFunc = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	mock.Server = httptest.NewServer(mock.RequestFunc)

	return mock
}

// Represents a new mock plugin for testing new relic
type MockPlugin struct {
	NewRelic MockNewRelic
	Plugin   *NewrelicPlugin
}

// Create a new plugin to test with
func NewMockPlugin() MockPlugin {
	mockPlugin := MockPlugin{}
	mockPlugin.Plugin = NewNewrelicPlugin("test.dev", "dummy-key", 1)
	mockPlugin.NewRelic = NewMockNewRelic()
	mockPlugin.Plugin.URL = mockPlugin.NewRelic.Server.URL
	mockPlugin.Plugin.AddComponent(NewPluginComponent("foo-component", "foo-guid", true))
	return mockPlugin
}

// Tests that a race condition doesn't occur when adding metrics and harvesting
func TestRaceCondition(t *testing.T) {
	mockPlugin := NewMockPlugin()
	go mockPlugin.Plugin.Run()

	start := time.Now().Unix()
	for ts := range time.Tick(10 * time.Millisecond) {
		if start+10 < ts.Unix() {
			break
		}
		mockPlugin.Plugin.ComponentModels[0].AddMetrica(NewMockMetrica())
	}
}
