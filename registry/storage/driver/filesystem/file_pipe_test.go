package filesystem

import (
	"github.com/fsnotify/fsnotify"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestReadAndWrite(t *testing.T) {
	file, err := ioutil.TempFile("", "*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		fileName := file.Name()
		file.Close()
		os.Remove(fileName)
	}()
	//bufio.NewWriter()
	count, err := file.Write([]byte("apple"))
	if count != len("apple") || err != nil {
		t.Fatalf("Length do not match: %d, err: %s", count, err)
	}
	fileR, err := os.Open(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fileR.Close() }()
	buff := make([]byte, 100)
	count, err = fileR.Read(buff)
	if count != len("apple") || err != nil {
		t.Fatalf("Error reading file count: %d, err: %s", count, err)
	}
	count, err = fileR.Read(buff)
	if count != 0 || err != io.EOF {
		t.Fatalf("Error reading file end, count: %d, err: %s", count, err)
	}
	count, err = file.Write([]byte("peach"))
	if count != len("peach") || err != nil {
		t.Fatalf("Length do not match: %d, err: %s", count, err)
	}
	count, err = fileR.Read(buff)
	if count != len("peach") || err != nil {
		t.Fatalf("Error reading file count: %d, err: %s", count, err)
	}
	count, err = fileR.Read(buff)
	if count != 0 || err != io.EOF {
		t.Fatalf("Error reading file end, count: %d, err: %s", count, err)
	}
	count, err = fileR.Read(buff)
	if count != 0 || err != io.EOF {
		t.Fatalf("Error reading file end, count: %d, err: %s", count, err)
	}

}

func TestReadAndWriteWithWatch(t *testing.T) {
	file, err := ioutil.TempFile("", "*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		fileName := file.Name()
		file.Close()
		os.Remove(fileName)
	}()
	//bufio.NewWriter()
	count, err := file.Write([]byte("apple"))
	if count != len("apple") || err != nil {
		t.Fatalf("Length do not match: %d, err: %s", count, err)
	}
	fileR, err := os.Open(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fileR.Close() }()
	buff := make([]byte, 100)
	count, err = fileR.Read(buff)
	if count != len("apple") || err != nil {
		t.Fatalf("Error reading file count: %d, err: %s", count, err)
	}
	count, err = fileR.Read(buff)
	if count != 0 || err != io.EOF {
		t.Fatalf("Error reading file end, count: %d, err: %s", count, err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()
	if err = watcher.Add(fileR.Name()); err != nil {
		t.Fatal(err)
	}

	count, err = file.Write([]byte("peach"))
	if count != len("peach") || err != nil {
		t.Fatalf("Length do not match: %d, err: %s", count, err)
	}

	select {
	case event, ok := <-watcher.Events:
		if !ok {
			t.Fatal("Should be ok")
		}
		if event.Op != fsnotify.Write {
			t.Fatalf("event should be write, %s", event)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Should receive write event almost instantly")
	}

	count, err = fileR.Read(buff)
	if count != len("peach") || err != nil {
		t.Fatalf("Error reading file count: %d, err: %s", count, err)
	}
	count, err = fileR.Read(buff)
	if count != 0 || err != io.EOF {
		t.Fatalf("Error reading file end, count: %d, err: %s", count, err)
	}
	count, err = fileR.Read(buff)
	if count != 0 || err != io.EOF {
		t.Fatalf("Error reading file end, count: %d, err: %s", count, err)
	}

}

func TestReadAndWriteAndMove(t *testing.T) {
	file, err := ioutil.TempFile("", "*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		fileName := file.Name()
		file.Close()
		os.Remove(fileName)
	}()
	//bufio.NewWriter()
	count, err := file.Write([]byte("apple"))
	if count != len("apple") || err != nil {
		t.Fatalf("Length do not match: %d, err: %s", count, err)
	}
	fileR, err := os.Open(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fileR.Close() }()
	buff := make([]byte, 100)
	count, err = fileR.Read(buff)
	if count != len("apple") || err != nil {
		t.Fatalf("Error reading file count: %d, err: %s", count, err)
	}
	count, err = fileR.Read(buff)
	if count != 0 || err != io.EOF {
		t.Fatalf("Error reading file end, count: %d, err: %s", count, err)
	}

	if err = os.Rename(file.Name(), file.Name()+".renamed"); err != nil {
		t.Fatal(err)
	}
	count, err = file.Write([]byte("peach"))
	if count != len("peach") || err != nil {
		t.Fatalf("Length do not match: %d, err: %s", count, err)
	}

	count, err = fileR.Read(buff)
	if count != len("peach") || err != nil {
		t.Fatalf("Error reading file count: %d, err: %s", count, err)
	}

}
