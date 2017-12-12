package bugsnag

import (
	"log"
	"os"
	"testing"

	"github.com/juju/loggo"
)

func TestNotifyReleaseStages(t *testing.T) {

	var testCases = []struct {
		stage      string
		configured []string
		notify     bool
		msg        string
	}{
		{
			stage:  "production",
			notify: true,
			msg:    "Should notify in all release stages by default",
		},
		{
			stage:      "production",
			configured: []string{"development", "production"},
			notify:     true,
			msg:        "Failed to notify in configured release stage",
		},
		{
			stage:      "staging",
			configured: []string{"development", "production"},
			notify:     false,
			msg:        "Failed to prevent notification in excluded release stage",
		},
	}

	for _, testCase := range testCases {
		Configure(Configuration{ReleaseStage: testCase.stage, NotifyReleaseStages: testCase.configured})

		if Config.notifyInReleaseStage() != testCase.notify {
			t.Error(testCase.msg)
		}
	}
}

func TestIsProjectPackage(t *testing.T) {

	Configure(Configuration{ProjectPackages: []string{
		"main",
		"star*",
		"example.com/a",
		"example.com/b/*",
		"example.com/c/*/*",
		"example.com/d/**",
		"example.com/e",
	}})

	var testCases = []struct {
		Path     string
		Included bool
	}{
		{"", false},
		{"main", true},
		{"runtime", false},

		{"star", true},
		{"sta", false},
		{"starred", true},
		{"star/foo", false},

		{"example.com/a", true},

		{"example.com/b", false},
		{"example.com/b/", true},
		{"example.com/b/foo", true},
		{"example.com/b/foo/bar", false},

		{"example.com/c/foo/bar", true},
		{"example.com/c/foo/bar/baz", false},

		{"example.com/d/foo/bar", true},
		{"example.com/d/foo/bar/baz", true},

		{"example.com/e", true},
	}

	for _, s := range testCases {
		if Config.isProjectPackage(s.Path) != s.Included {
			t.Error("literal project package doesn't work:", s.Path, s.Included)
		}
	}
}

func TestStripProjectPackage(t *testing.T) {

	Configure(Configuration{ProjectPackages: []string{
		"main",
		"star*",
		"example.com/a",
		"example.com/b/*",
		"example.com/c/**",
	}})

	gopath := os.Getenv("GOPATH")
	var testCases = []struct {
		File     string
		Stripped string
	}{
		{"main.go", "main.go"},
		{"runtime.go", "runtime.go"},
		{"star.go", "star.go"},

		{"example.com/a/foo.go", "foo.go"},

		{"example.com/b/foo/bar.go", "foo/bar.go"},
		{"example.com/b/foo.go", "foo.go"},

		{"example.com/x/a/b/foo.go", "example.com/x/a/b/foo.go"},

		{"example.com/c/a/b/foo.go", "a/b/foo.go"},

		{gopath + "/src/runtime.go", "runtime.go"},
		{gopath + "/src/example.com/a/foo.go", "foo.go"},
		{gopath + "/src/example.com/x/a/b/foo.go", "example.com/x/a/b/foo.go"},
		{gopath + "/src/example.com/c/a/b/foo.go", "a/b/foo.go"},
	}

	for _, tc := range testCases {
		if s := Config.stripProjectPackages(tc.File); s != tc.Stripped {
			t.Error("stripProjectPackage did not remove expected path:", tc.File, tc.Stripped, "was:", s)
		}
	}
}

func TestStripCustomSourceRoot(t *testing.T) {
	Configure(Configuration{
		ProjectPackages: []string{
			"main",
			"star*",
			"example.com/a",
			"example.com/b/*",
			"example.com/c/**",
		},
		SourceRoot: "/Users/bob/code/go/src/",
	})
	var testCases = []struct {
		File     string
		Stripped string
	}{
		{"main.go", "main.go"},
		{"runtime.go", "runtime.go"},
		{"star.go", "star.go"},

		{"example.com/a/foo.go", "foo.go"},

		{"example.com/b/foo/bar.go", "foo/bar.go"},
		{"example.com/b/foo.go", "foo.go"},

		{"example.com/x/a/b/foo.go", "example.com/x/a/b/foo.go"},

		{"example.com/c/a/b/foo.go", "a/b/foo.go"},

		{"/Users/bob/code/go/src/runtime.go", "runtime.go"},
		{"/Users/bob/code/go/src/example.com/a/foo.go", "foo.go"},
		{"/Users/bob/code/go/src/example.com/x/a/b/foo.go", "example.com/x/a/b/foo.go"},
		{"/Users/bob/code/go/src/example.com/c/a/b/foo.go", "a/b/foo.go"},
	}

	for _, tc := range testCases {
		if s := Config.stripProjectPackages(tc.File); s != tc.Stripped {
			t.Error("stripProjectPackage did not remove expected path:", tc.File, tc.Stripped, "was:", s)
		}
	}
}

type LoggoWrapper struct {
	loggo.Logger
}

func (lw *LoggoWrapper) Printf(format string, v ...interface{}) {
	lw.Logger.Warningf(format, v...)
}

func TestConfiguringCustomLogger(t *testing.T) {

	l1 := log.New(os.Stdout, "", log.Lshortfile)

	l2 := &LoggoWrapper{loggo.GetLogger("test")}

	var testCases = []struct {
		config Configuration
		notify bool
		msg    string
	}{
		{
			config: Configuration{ReleaseStage: "production", NotifyReleaseStages: []string{"development", "production"}, Logger: l1},
			notify: true,
			msg:    "Failed to assign log.Logger",
		},
		{
			config: Configuration{ReleaseStage: "production", NotifyReleaseStages: []string{"development", "production"}, Logger: l2},
			notify: true,
			msg:    "Failed to assign LoggoWrapper",
		},
	}

	for _, testCase := range testCases {
		Configure(testCase.config)

		// call printf just to illustrate it is present as the compiler does most of the hard work
		testCase.config.Logger.Printf("hello %s", "bugsnag")

	}
}
