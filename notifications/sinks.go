package notifications

import (
	"container/list"
	"fmt"
	"sync"

	events "github.com/docker/go-events"
	"github.com/sirupsen/logrus"
)

// eventQueue accepts all messages into a queue for asynchronous consumption
// by a sink. It is unbounded and thread safe but the sink must be reliable or
// events will be dropped.
type eventQueue struct {
	sink      events.Sink
	events    *list.List
	listeners []eventQueueListener
	cond      *sync.Cond
	mu        sync.Mutex
	closed    bool
}

// eventQueueListener is called when various events happen on the queue.
type eventQueueListener interface {
	ingress(event events.Event)
	egress(event events.Event)
}

// newEventQueue returns a queue to the provided sink. If the updater is non-
// nil, it will be called to update pending metrics on ingress and egress.
func newEventQueue(sink events.Sink, listeners ...eventQueueListener) *eventQueue {
	eq := eventQueue{
		sink:      sink,
		events:    list.New(),
		listeners: listeners,
	}

	eq.cond = sync.NewCond(&eq.mu)
	go eq.run()
	return &eq
}

// Write accepts the events into the queue, only failing if the queue has
// beend closed.
func (eq *eventQueue) Write(event events.Event) error {
	eq.mu.Lock()
	defer eq.mu.Unlock()

	if eq.closed {
		return ErrSinkClosed
	}

	for _, listener := range eq.listeners {
		listener.ingress(event)
	}
	eq.events.PushBack(event)
	eq.cond.Signal() // signal waiters

	return nil
}

// Close shuts down the event queue, flushing
func (eq *eventQueue) Close() error {
	eq.mu.Lock()
	defer eq.mu.Unlock()

	if eq.closed {
		return fmt.Errorf("eventqueue: already closed")
	}

	// set closed flag
	eq.closed = true
	eq.cond.Signal() // signal flushes queue
	eq.cond.Wait()   // wait for signal from last flush

	return eq.sink.Close()
}

// run is the main goroutine to flush events to the target sink.
func (eq *eventQueue) run() {
	for {
		event := eq.next()

		if event == nil {
			return // nil block means event queue is closed.
		}

		if err := eq.sink.Write(event); err != nil {
			logrus.Warnf("eventqueue: error writing events to %v, these events will be lost: %v", eq.sink, err)
		}

		for _, listener := range eq.listeners {
			listener.egress(event)
		}
	}
}

// next encompasses the critical section of the run loop. When the queue is
// empty, it will block on the condition. If new data arrives, it will wake
// and return a block. When closed, a nil slice will be returned.
func (eq *eventQueue) next() events.Event {
	eq.mu.Lock()
	defer eq.mu.Unlock()

	for eq.events.Len() < 1 {
		if eq.closed {
			eq.cond.Broadcast()
			return nil
		}

		eq.cond.Wait()
	}

	front := eq.events.Front()
	block := front.Value.(events.Event)
	eq.events.Remove(front)

	return block
}

// ignoredSink discards events with ignored target media types and actions.
// passes the rest along.
type ignoredSink struct {
	events.Sink
	ignoreMediaTypes map[string]bool
	ignoreActions    map[string]bool
}

func newIgnoredSink(sink events.Sink, ignored []string, ignoreActions []string) events.Sink {
	if len(ignored) == 0 {
		return sink
	}

	ignoredMap := make(map[string]bool)
	for _, mediaType := range ignored {
		ignoredMap[mediaType] = true
	}

	ignoredActionsMap := make(map[string]bool)
	for _, action := range ignoreActions {
		ignoredActionsMap[action] = true
	}

	return &ignoredSink{
		Sink:             sink,
		ignoreMediaTypes: ignoredMap,
		ignoreActions:    ignoredActionsMap,
	}
}

// Write discards events with ignored target media types and passes the rest
// along.
func (imts *ignoredSink) Write(event events.Event) error {
	if imts.ignoreMediaTypes[event.(Event).Target.MediaType] || imts.ignoreActions[event.(Event).Action] {
		return nil
	}

	return imts.Sink.Write(event)
}

func (imts *ignoredSink) Close() error {
	return nil
}
