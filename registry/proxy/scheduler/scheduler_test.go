package scheduler

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
)

func TestSchedule(t *testing.T) {
	timeUnit := time.Millisecond
	remainingRepos := map[string]bool{
		"testBlob1": true,
		"testBlob2": true,
		"testBlob3": true,
	}

	s := New(context.Background(), inmemory.New(), "/ttl")
	deleteFunc := func(repoName string) error {
		if len(remainingRepos) == 0 {
			t.Fatalf("Incorrect expiry count")
		}
		_, ok := remainingRepos[repoName]
		if !ok {
			t.Fatalf("Trying to remove nonexistant repo: %s", repoName)
		}
		fmt.Println("removing", repoName)
		delete(remainingRepos, repoName)

		return nil
	}
	s.OnBlobExpire(deleteFunc)
	err := s.Start()
	if err != nil {
		t.Fatalf("Error starting ttlExpirationScheduler: %s", err)
	}

	s.AddBlob("testBlob1", 3*timeUnit)
	s.AddBlob("testBlob2", 1*timeUnit)

	func() {
		s.AddBlob("testBlob3", 1*timeUnit)

	}()

	// Ensure all repos are deleted
	<-time.After(10 * timeUnit)
	if len(remainingRepos) != 0 {
		t.Fatalf("Repositories remaining: %#v", remainingRepos)
	}
}

func TestRestoreOld(t *testing.T) {
	remainingRepos := map[string]bool{
		"testBlob1": true,
		"oldRepo":   true,
	}

	deleteFunc := func(repoName string) error {
		if repoName == "oldRepo" && len(remainingRepos) == 3 {
			t.Errorf("oldRepo should be removed first")
		}
		_, ok := remainingRepos[repoName]
		if !ok {
			t.Fatalf("Trying to remove nonexistant repo: %s", repoName)
		}
		delete(remainingRepos, repoName)
		return nil
	}

	timeUnit := time.Millisecond
	serialized, err := json.Marshal(&map[string]schedulerEntry{
		"testBlob1": schedulerEntry{
			ExpiryDate: time.Now().Add(1 * timeUnit),
			Key:        "testBlob1",
			EntryType:  0,
		},
		"oldRepo": schedulerEntry{
			ExpiryDate: time.Now().Add(-3 * timeUnit), // TTL passed, should be removed first
			Key:        "oldRepo",
			EntryType:  0,
		},
	})
	if err != nil {
		t.Fatalf("Error serializing test data: %s", err.Error())
	}

	ctx := context.Background()
	pathToStatFile := "/ttl"
	fs := inmemory.New()
	err = fs.PutContent(ctx, pathToStatFile, serialized)
	if err != nil {
		t.Fatal("Unable to write serialized data to fs")
	}
	s := New(context.Background(), fs, "/ttl")
	s.OnBlobExpire(deleteFunc)
	err = s.Start()
	if err != nil {
		t.Fatalf("Error starting ttlExpirationScheduler: %s", err)
	}

	<-time.After(5 * timeUnit)
	if len(remainingRepos) != 0 {
		t.Fatalf("Repositories remaining: %#v", remainingRepos)
	}
}

func TestStopRestore(t *testing.T) {
	timeUnit := time.Millisecond
	remainingRepos := map[string]bool{
		"testBlob1": true,
		"testBlob2": true,
	}
	deleteFunc := func(repoName string) error {
		delete(remainingRepos, repoName)
		return nil
	}

	fs := inmemory.New()
	pathToStateFile := "/ttl"
	s := New(context.Background(), fs, pathToStateFile)
	s.OnBlobExpire(deleteFunc)

	err := s.Start()
	if err != nil {
		t.Fatalf(err.Error())
	}
	s.AddBlob("testBlob1", 300*timeUnit)
	s.AddBlob("testBlob2", 100*timeUnit)

	// Start and stop before all operations complete
	// state will be written to fs
	s.Stop()
	time.Sleep(10 * time.Millisecond)

	// v2 will restore state from fs
	s2 := New(context.Background(), fs, pathToStateFile)
	s2.OnBlobExpire(deleteFunc)
	err = s2.Start()
	if err != nil {
		t.Fatalf("Error starting v2: %s", err.Error())
	}

	<-time.After(500 * timeUnit)
	if len(remainingRepos) != 0 {
		t.Fatalf("Repositories remaining: %#v", remainingRepos)
	}

}

func TestDoubleStart(t *testing.T) {
	s := New(context.Background(), inmemory.New(), "/ttl")
	err := s.Start()
	if err != nil {
		t.Fatalf("Unable to start scheduler")
	}
	fmt.Printf("%#v", s)
	err = s.Start()
	if err == nil {
		t.Fatalf("Scheduler started twice without error")
	}
}
