package logrustash_test

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/bshuster-repo/logrus-logstash-hook"
	"github.com/sirupsen/logrus"
)

func TestEntryIsNotChangedByLogstashFormatter(t *testing.T) {
	buffer := bytes.NewBufferString("")
	bufferOut := bytes.NewBufferString("")

	log := logrus.New()
	log.Out = bufferOut

	hook := logrustash.New(buffer, logrustash.DefaultFormatter(logrus.Fields{"NICKNAME": ""}))
	log.Hooks.Add(hook)

	log.Info("hello world")

	if !strings.Contains(buffer.String(), "NICKNAME\":") {
		t.Errorf("expected logstash message to have '%s': %#v", "NICKNAME\":", buffer.String())
	}
	if strings.Contains(bufferOut.String(), "NICKNAME\":") {
		t.Errorf("expected main logrus message to not have '%s': %#v", "NICKNAME\":", buffer.String())
	}
}

func TestTimestampFormatKitchen(t *testing.T) {
	log := logrus.New()
	buffer := bytes.NewBufferString("")
	hook := logrustash.New(buffer, logrustash.LogstashFormatter{
		Formatter: &logrus.JSONFormatter{
			FieldMap: logrus.FieldMap{
				logrus.FieldKeyTime: "@timestamp",
				logrus.FieldKeyMsg:  "message",
			},
			TimestampFormat: time.Kitchen,
		},
		Fields: logrus.Fields{"HOSTNAME": "localhost", "USERNAME": "root"},
	})
	log.Hooks.Add(hook)

	log.Error("this is an error message!")
	mTime := time.Now()
	expected := fmt.Sprintf(`{"@timestamp":"%s","HOSTNAME":"localhost","USERNAME":"root","level":"error","message":"this is an error message!"}
`, mTime.Format(time.Kitchen))
	if buffer.String() != expected {
		t.Errorf("expected JSON to be '%#v' but got '%#v'", expected, buffer.String())
	}
}

func TestTextFormatLogstash(t *testing.T) {
	log := logrus.New()
	buffer := bytes.NewBufferString("")
	hook := logrustash.New(buffer, logrustash.LogstashFormatter{
		Formatter: &logrus.TextFormatter{
			TimestampFormat: time.Kitchen,
		},
		Fields: logrus.Fields{"HOSTNAME": "localhost", "USERNAME": "root"},
	})
	log.Hooks.Add(hook)

	log.Warning("this is a warning message!")
	mTime := time.Now()
	expected := fmt.Sprintf(`time="%s" level=warning msg="this is a warning message!" HOSTNAME=localhost USERNAME=root
`, mTime.Format(time.Kitchen))
	if buffer.String() != expected {
		t.Errorf("expected JSON to be '%#v' but got '%#v'", expected, buffer.String())
	}
}

// Github issue #39
func TestLogWithFieldsDoesNotOverrideHookFields(t *testing.T) {
	log := logrus.New()
	buffer := bytes.NewBufferString("")
	hook := logrustash.New(buffer, logrustash.LogstashFormatter{
		Formatter: &logrus.JSONFormatter{},
		Fields:    logrus.Fields{},
	})
	log.Hooks.Add(hook)
	log.WithField("animal", "walrus").Info("bla")
	attr := "\"animal\":\"walrus\""
	if !strings.Contains(buffer.String(), attr) {
		t.Errorf("expected to have '%s' in '%s'", attr, buffer.String())
	}
	buffer.Reset()
	log.Info("hahaha")
	if strings.Contains(buffer.String(), attr) {
		t.Errorf("expected not to have '%s' in '%s'", attr, buffer.String())
	}
}

func TestDefaultFormatterNotOverrideMyLogstashFieldsValues(t *testing.T) {
	formatter := logrustash.DefaultFormatter(logrus.Fields{"@version": "2", "type": "mylogs"})

	dataBytes, err := formatter.Format(&logrus.Entry{Data: logrus.Fields{}})
	if err != nil {
		t.Errorf("expected Format to not return error: %s", err)
	}

	expected := []string{
		`"@version":"2"`,
		`"type":"mylogs"`,
	}

	for _, expField := range expected {
		if !strings.Contains(string(dataBytes), expField) {
			t.Errorf("expected '%s' to be in '%s'", expField, string(dataBytes))
		}
	}
}

func TestDefaultFormatterLogstashFields(t *testing.T) {
	formatter := logrustash.DefaultFormatter(logrus.Fields{})

	dataBytes, err := formatter.Format(&logrus.Entry{Data: logrus.Fields{}})
	if err != nil {
		t.Errorf("expected Format to not return error: %s", err)
	}

	expected := []string{
		`"@version":"1"`,
		`"type":"log"`,
	}

	for _, expField := range expected {
		if !strings.Contains(string(dataBytes), expField) {
			t.Errorf("expected '%s' to be in '%s'", expField, string(dataBytes))
		}
	}
}

// UDP will never fail because it's connectionless.
// That's why I am using it for this integration tests just to make sure
// it won't fail when a data is written.
func TestUDPWritter(t *testing.T) {
	log := logrus.New()
	conn, err := net.Dial("udp", ":8282")
	if err != nil {
		t.Errorf("expected Dial to not return error: %s", err)
	}
	hook := logrustash.New(conn, &logrus.JSONFormatter{})
	log.Hooks.Add(hook)

	log.Info("this is an information message")
}
