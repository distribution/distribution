package base

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
)

func TestRegulatorEnterExit(t *testing.T) {
	const limit = 500

	r := NewRegulator(nil, limit).(*regulator)

	for range 50 {
		run := make(chan struct{})

		var firstGroupReady sync.WaitGroup
		var firstGroupDone sync.WaitGroup
		firstGroupReady.Add(limit)
		firstGroupDone.Add(limit)
		for range limit {
			go func() {
				r.enter()
				firstGroupReady.Done()
				<-run
				r.exit()
				firstGroupDone.Done()
			}()
		}
		firstGroupReady.Wait()

		// now we exhausted all the limit, let's run a little bit more
		var secondGroupReady sync.WaitGroup
		var secondGroupDone sync.WaitGroup
		for range 50 {
			secondGroupReady.Add(1)
			secondGroupDone.Go(func() {
				secondGroupReady.Done()
				r.enter()
				r.exit()
			})
		}
		secondGroupReady.Wait()

		// allow the first group to return resources
		close(run)

		done := make(chan struct{})
		go func() {
			secondGroupDone.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("some r.enter() are still locked")
		}

		firstGroupDone.Wait()

		if r.available != limit {
			t.Fatalf("r.available: got %d, want %d", r.available, limit)
		}
	}
}

type nameListDriver struct {
	storagedriver.StorageDriver
	list func(context.Context, string) ([]string, error)
}

func (d *nameListDriver) Name() string { return "names" }

func (d *nameListDriver) ListNames(ctx context.Context, path string, _ storagedriver.StorageDriver) ([]string, error) {
	return d.list(ctx, path)
}

func TestRegulatorListNames(t *testing.T) {
	var r *regulator
	wantErr := errors.New("listing failed")
	d := &nameListDriver{list: func(context.Context, string) ([]string, error) {
		r.L.Lock()
		defer r.L.Unlock()
		if r.available != 0 {
			t.Error("ListNames did not acquire a regulator permit")
		}
		return nil, wantErr
	}}
	r = NewRegulator(d, 1).(*regulator)
	for range 2 {
		_, err := storagedriver.ListNames(context.Background(), r, "/tags")
		if err != wantErr {
			t.Fatalf("got %v, want %v", err, wantErr)
		}
		if r.available != 1 {
			t.Fatal("ListNames did not release its permit after an error")
		}
	}
}

func TestGetLimitFromParameter(t *testing.T) {
	tests := []struct {
		Input    any
		Expected uint64
		Min      uint64
		Default  uint64
		Err      error
	}{
		{"foo", 0, 5, 5, fmt.Errorf("parameter must be an integer, 'foo' invalid")},
		{"50", 50, 5, 5, nil},
		{"5", 25, 25, 50, nil}, // lower than Min returns Min
		{nil, 50, 25, 50, nil}, // nil returns default
		{812, 812, 25, 50, nil},
	}

	for _, item := range tests {
		t.Run(fmt.Sprint(item.Input), func(t *testing.T) {
			actual, err := GetLimitFromParameter(item.Input, item.Min, item.Default)

			if err != nil && item.Err != nil && err.Error() != item.Err.Error() {
				t.Fatalf("GetLimitFromParameter error, expected %#v got %#v", item.Err, err)
			}

			if actual != item.Expected {
				t.Fatalf("GetLimitFromParameter result error, expected %d got %d", item.Expected, actual)
			}
		})
	}
}
