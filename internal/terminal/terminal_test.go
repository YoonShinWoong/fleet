package terminal

import (
	"bytes"
	"testing"
	"time"
)

func TestShellRoundTrip(t *testing.T) {
	m := NewManager(4096, nil)
	term, err := m.Create(Options{Name: "t1", Command: "cat", Shell: "/bin/sh", Size: Size{Cols: 80, Rows: 24}})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := term.Attach(Size{Cols: 80, Rows: 24})
	defer c.Close()
	if err := c.Write([]byte("hello-fleet\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if bytes.Count(term.Scrollback(), []byte("hello-fleet")) >= 2 { // echo + cat
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n := bytes.Count(term.Scrollback(), []byte("hello-fleet")); n < 2 {
		t.Fatalf("expected echo and cat output, got %d occurrence(s): %q", n, term.Scrollback())
	}
	if !term.Info().Alive || term.Info().Clients != 1 {
		t.Fatalf("info: %+v", term.Info())
	}
	if err := m.Kill("t1"); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-c.Exited():
		_ = code
	case <-time.After(3 * time.Second):
		t.Fatal("client was not told about exit")
	}
	if _, ok := m.Get("t1"); ok {
		t.Fatal("terminal still listed after kill")
	}
}

func TestRingKeepsTail(t *testing.T) {
	r := newRing(5)
	r.Write([]byte("abcdefg"))
	if got := string(r.Bytes()); got != "cdefg" {
		t.Fatalf("got %q", got)
	}
}

func TestSmallestClientWins(t *testing.T) {
	m := NewManager(1024, nil)
	term, err := m.Create(Options{Name: "t2", Command: "sleep 5", Shell: "/bin/sh"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Kill("t2")
	a, _ := term.Attach(Size{Cols: 200, Rows: 50})
	b, _ := term.Attach(Size{Cols: 100, Rows: 60})
	time.Sleep(150 * time.Millisecond)
	if sz := term.Info().Size; sz != (Size{Cols: 100, Rows: 50}) {
		t.Fatalf("size %+v", sz)
	}
	b.Close()
	time.Sleep(50 * time.Millisecond)
	if sz := term.Info().Size; sz != (Size{Cols: 200, Rows: 50}) {
		t.Fatalf("size after detach %+v", sz)
	}
	a.Close()
}
