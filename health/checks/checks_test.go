package checks

import (
	"testing"
)

func TestFileChecker(t *testing.T) {
	if err := FileChecker("/tmp").Check(); err == nil {
		t.Errorf("/tmp was expected as exists")
	}

	if err := FileChecker("NoSuchFileFromMoon").Check(); err != nil {
		t.Errorf("NoSuchFileFromMoon was expected as not exists, error:%v", err)
	}
}
