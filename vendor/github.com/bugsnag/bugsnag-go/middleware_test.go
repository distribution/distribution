package bugsnag

import (
	"bytes"
	"fmt"
	"github.com/bugsnag/bugsnag-go/errors"
	"log"
	"reflect"
	"testing"
)

func TestMiddlewareOrder(t *testing.T) {

	err := fmt.Errorf("test")
	event, config := newEvent(errors.New(err, 1), make([]interface{}, 0), &defaultNotifier)

	result := make([]int, 0, 7)
	stack := middlewareStack{}
	stack.OnBeforeNotify(func(e *Event, c *Configuration) error {
		result = append(result, 2)
		return nil
	})
	stack.OnBeforeNotify(func(e *Event, c *Configuration) error {
		result = append(result, 1)
		return nil
	})
	stack.OnBeforeNotify(func(e *Event, c *Configuration) error {
		result = append(result, 0)
		return nil
	})

	stack.Run(event, config, func() error {
		result = append(result, 3)
		return nil
	})

	if !reflect.DeepEqual(result, []int{0, 1, 2, 3}) {
		t.Errorf("unexpected middleware order %v", result)
	}
}

func TestBeforeNotifyReturnErr(t *testing.T) {

	stack := middlewareStack{}
	err := fmt.Errorf("test")
	event, config := newEvent(errors.New(err, 1), make([]interface{}, 0), &defaultNotifier)

	stack.OnBeforeNotify(func(e *Event, c *Configuration) error {
		return err
	})

	called := false

	e := stack.Run(event, config, func() error {
		called = true
		return nil
	})

	if e != err {
		t.Errorf("Middleware didn't return the error")
	}

	if called == true {
		t.Errorf("Notify was called when BeforeNotify returned False")
	}
}

func TestBeforeNotifyPanic(t *testing.T) {

	stack := middlewareStack{}
	err := fmt.Errorf("test")
	event, _ := newEvent(errors.New(err, 1), make([]interface{}, 0), &defaultNotifier)

	stack.OnBeforeNotify(func(e *Event, c *Configuration) error {
		panic("oops")
	})

	called := false
	b := &bytes.Buffer{}

	stack.Run(event, &Configuration{Logger: log.New(b, log.Prefix(), 0)}, func() error {
		called = true
		return nil
	})

	logged := b.String()

	if logged != "bugsnag/middleware: unexpected panic: oops\n" {
		t.Errorf("Logged: %s", logged)
	}

	if called == false {
		t.Errorf("Notify was not called when BeforeNotify panicked")
	}
}
