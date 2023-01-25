package notifications

import (
	"reflect"
	"sync"
	"time"

	events "github.com/docker/go-events"

	"github.com/sirupsen/logrus"

	"testing"
)

func TestEventQueue(t *testing.T) {
	const nevents = 1000
	var ts testSink
	metrics := newSafeMetrics("")
	eq := newEventQueue(
		// delayed sync simulates destination slower than channel comms
		&delayedSink{
			Sink:  &ts,
			delay: time.Millisecond * 1,
		}, metrics.eventQueueListener())

	var wg sync.WaitGroup
	var event events.Event
	for i := 1; i <= nevents; i++ {
		event = createTestEvent("push", "library/test", "blob")
		wg.Add(1)
		go func(event events.Event) {
			if err := eq.Write(event); err != nil {
				t.Errorf("error writing event block: %v", err)
			}
			wg.Done()
		}(event)
	}

	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}
	checkClose(t, eq)

	ts.mu.Lock()
	defer ts.mu.Unlock()
	metrics.Lock()
	defer metrics.Unlock()

	if ts.count != nevents {
		t.Fatalf("events did not make it to the sink: %d != %d", ts.count, 1000)
	}

	if !ts.closed {
		t.Fatalf("sink should have been closed")
	}

	if metrics.Events != nevents {
		t.Fatalf("unexpected ingress count: %d != %d", metrics.Events, nevents)
	}

	if metrics.Pending != 0 {
		t.Fatalf("unexpected egress count: %d != %d", metrics.Pending, 0)
	}
}

func TestIgnoredSink(t *testing.T) {
	blob := createTestEvent("push", "library/test", "blob")
	manifest := createTestEvent("pull", "library/test", "manifest")

	type testcase struct {
		ignoreMediaTypes []string
		ignoreActions    []string
		expected         events.Event
	}

	cases := []testcase{
		{nil, nil, blob},
		{[]string{"other"}, []string{"other"}, blob},
		{[]string{"blob", "manifest"}, []string{"other"}, nil},
		{[]string{"other"}, []string{"pull"}, blob},
		{[]string{"other"}, []string{"pull", "push"}, nil},
	}

	for _, c := range cases {
		ts := &testSink{}
		s := newIgnoredSink(ts, c.ignoreMediaTypes, c.ignoreActions)

		if err := s.Write(blob); err != nil {
			t.Fatalf("error writing event: %v", err)
		}

		ts.mu.Lock()
		if !reflect.DeepEqual(ts.event, c.expected) {
			t.Fatalf("unexpected event: %#v != %#v", ts.event, c.expected)
		}
		ts.mu.Unlock()
	}

	cases = []testcase{
		{nil, nil, manifest},
		{[]string{"other"}, []string{"other"}, manifest},
		{[]string{"blob"}, []string{"other"}, manifest},
		{[]string{"blob", "manifest"}, []string{"other"}, nil},
		{[]string{"other"}, []string{"push"}, manifest},
		{[]string{"other"}, []string{"pull", "push"}, nil},
	}

	for _, c := range cases {
		ts := &testSink{}
		s := newIgnoredSink(ts, c.ignoreMediaTypes, c.ignoreActions)

		if err := s.Write(manifest); err != nil {
			t.Fatalf("error writing event: %v", err)
		}

		ts.mu.Lock()
		if !reflect.DeepEqual(ts.event, c.expected) {
			t.Fatalf("unexpected event: %#v != %#v", ts.event, c.expected)
		}
		ts.mu.Unlock()
	}
}

type testSink struct {
	event  events.Event
	count  int
	mu     sync.Mutex
	closed bool
}

func (ts *testSink) Write(event events.Event) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.event = event
	ts.count++
	return nil
}

func (ts *testSink) Close() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.closed = true

	logrus.Infof("closing testSink")
	return nil
}

type delayedSink struct {
	events.Sink
	delay time.Duration
}

func (ds *delayedSink) Write(event events.Event) error {
	time.Sleep(ds.delay)
	return ds.Sink.Write(event)
}

func checkClose(t *testing.T, sink events.Sink) {
	if err := sink.Close(); err != nil {
		t.Fatalf("unexpected error closing: %v", err)
	}

	// second close should not crash but should return an error.
	if err := sink.Close(); err == nil {
		t.Fatalf("no error on double close")
	}

	// Write after closed should be an error
	if err := sink.Write(Event{}); err == nil {
		t.Fatalf("write after closed did not have an error")
	} else if err != ErrSinkClosed {
		t.Fatalf("error should be ErrSinkClosed")
	}
}
