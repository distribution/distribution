package checks

import (
	"context"
	"testing"
)

func TestFileChecker(t *testing.T) {
	dir := t.TempDir()
	if err := FileChecker(dir).Check(context.Background()); err == nil {
		t.Errorf("%s was expected as exists", dir)
	}

	if err := FileChecker("NoSuchFileFromMoon").Check(context.Background()); err != nil {
		t.Errorf("NoSuchFileFromMoon was expected as not exists, error:%v", err)
	}
}

func TestHTTPChecker(t *testing.T) {
	if err := HTTPChecker("https://www.google.cybertron", 200, 0, nil).Check(context.Background()); err == nil {
		t.Errorf("Google on Cybertron was expected as not exists")
	}

	if err := HTTPChecker("https://www.google.pt", 200, 0, nil).Check(context.Background()); err != nil {
		t.Errorf("Google at Portugal was expected as exists, error:%v", err)
	}
}
