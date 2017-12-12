package errors

import (
	"bytes"
	"fmt"
	"io"
	"runtime/debug"
	"testing"
)

func TestStackFormatMatches(t *testing.T) {

	defer func() {
		err := recover()
		if err != 'a' {
			t.Fatal(err)
		}

		bs := [][]byte{Errorf("hi").Stack(), debug.Stack()}

		// Ignore the first line (as it contains the PC of the .Stack() call)
		bs[0] = bytes.SplitN(bs[0], []byte("\n"), 2)[1]
		bs[1] = bytes.SplitN(bs[1], []byte("\n"), 2)[1]

		if bytes.Compare(bs[0], bs[1]) != 0 {
			t.Errorf("Stack didn't match")
			t.Errorf("%s", bs[0])
			t.Errorf("%s", bs[1])
		}
	}()

	a()
}

func TestSkipWorks(t *testing.T) {

	defer func() {
		err := recover()
		if err != 'a' {
			t.Fatal(err)
		}

		bs := [][]byte{New("hi", 2).Stack(), debug.Stack()}

		if !bytes.HasSuffix(bs[1], bs[0]) {
			t.Errorf("Stack didn't match")
			t.Errorf("%s", bs[0])
			t.Errorf("%s", bs[1])
		}
	}()

	a()
}

type testErrorWithStackFrames struct {
	Err *Error
}

func (tews *testErrorWithStackFrames) StackFrames() []StackFrame {
	return tews.Err.StackFrames()
}

func (tews *testErrorWithStackFrames) Error() string {
	return tews.Err.Error()
}

func TestNewError(t *testing.T) {

	e := func() error {
		return New("hi", 1)
	}()

	if e.Error() != "hi" {
		t.Errorf("Constructor with a string failed")
	}

	if New(fmt.Errorf("yo"), 0).Error() != "yo" {
		t.Errorf("Constructor with an error failed")
	}

	if New(e, 0) != e {
		t.Errorf("Constructor with an Error failed")
	}

	if New(nil, 0).Error() != "<nil>" {
		t.Errorf("Constructor with nil failed")
	}

	err := New("foo", 0)
	tews := &testErrorWithStackFrames{
		Err: err,
	}

	if bytes.Compare(New(tews, 0).Stack(), err.Stack()) != 0 {
		t.Errorf("Constructor with ErrorWithStackFrames failed")
	}
}

func ExampleErrorf() {
	for i := 1; i <= 2; i++ {
		if i%2 == 1 {
			e := Errorf("can only halve even numbers, got %d", i)
			fmt.Printf("Error: %+v", e)
		}
	}
	// Output:
	// Error: can only halve even numbers, got 1
}

func ExampleNew() {
	// Wrap io.EOF with the current stack-trace and return it
	e := New(io.EOF, 0)
	fmt.Printf("%+v", e)
	// Output:
	// EOF
}

func ExampleNew_skip() {
	defer func() {
		if err := recover(); err != nil {
			// skip 1 frame (the deferred function) and then return the wrapped err
			err = New(err, 1)
		}
	}()
}

func ExampleError_Stack() {
	e := New("Oh noes!", 1)
	fmt.Printf("Error: %s\n", e.Error())
	fmt.Printf("Stack is %d bytes", len(e.Stack()))
	// Output:
	// Error: Oh noes!
	// Stack is 589 bytes
}

func a() error {
	b(5)
	return nil
}

func b(i int) {
	c()
}

func c() {
	panic('a')
}
