package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

// TestMemo proves the lazy getter builds exactly once even under concurrent
// callers and that errors are cached (not retried).
func TestMemo(t *testing.T) {
	t.Run("builds once and caches", func(t *testing.T) {
		calls := 0
		g := newMemo(func() (int, error) {
			calls++
			return 42, nil
		})

		v1, err := g()
		assertNoErr(t, err)
		v2, err := g()
		assertNoErr(t, err)

		if v1 != 42 || v2 != 42 {
			t.Fatalf("want 42, got %d / %d", v1, v2)
		}
		if calls != 1 {
			t.Fatalf("want 1 build, got %d", calls)
		}
	})

	t.Run("caches error without retry", func(t *testing.T) {
		calls := 0
		want := errors.New("boom")
		g := newMemo(func() (int, error) {
			calls++
			return 0, want
		})

		_, err := g()
		if !errors.Is(err, want) {
			t.Fatalf("want err boom, got %v", err)
		}
		_, err = g()
		if !errors.Is(err, want) {
			t.Fatalf("want cached err boom, got %v", err)
		}
		if calls != 1 {
			t.Fatalf("failed build should not retry; got %d calls", calls)
		}
	})

	t.Run("concurrent callers share one build", func(t *testing.T) {
		calls := 0
		var mu sync.Mutex
		g := newMemo(func() (int, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			return 7, nil
		})

		const n = 32
		var wg sync.WaitGroup
		wg.Add(n)
		start := make(chan struct{})
		for range n {
			go func() {
				defer wg.Done()
				<-start
				_, _ = g()
			}()
		}
		close(start)
		wg.Wait()

		if calls != 1 {
			t.Fatalf("want 1 build under concurrency, got %d", calls)
		}
	})
}

// TestErrNotConfigured verifies a container built without a factory fails loudly
// instead of nil-derefing.
func TestErrNotConfigured(t *testing.T) {
	svc := NewContainer(ContainerFactories{})
	_, err := svc.AccountClient(context.Background())
	if err == nil {
		t.Fatal("want ErrNotConfigured, got nil")
	}
	if _, ok := err.(ErrNotConfigured); !ok {
		t.Fatalf("want ErrNotConfigured type, got %T", err)
	}
}

// TestPresenter covers Result/Success/Error across human and JSON formats.
func TestPresenter(t *testing.T) {
	t.Run("human Result defaults to indented JSON", func(t *testing.T) {
		out := &bytes.Buffer{}
		p := NewPresenter(out, OutputHuman, &bytes.Buffer{}, nil)
		p.Result(LoginResult{User: "x"})
		assertContains(t, out.String(), `"user": "x"`)
	})

	t.Run("JSON Result marshals compact", func(t *testing.T) {
		out := &bytes.Buffer{}
		p := NewPresenter(out, OutputJSON, &bytes.Buffer{}, nil)
		p.Result(LoginResult{User: "y"})
		assertJSONEq(t, `{"user":"y"}`, out.String())
	})

	t.Run("human Success prints line", func(t *testing.T) {
		out := &bytes.Buffer{}
		p := NewPresenter(out, OutputHuman, &bytes.Buffer{}, nil)
		p.Success("done")
		assertJSONEq(t, "done", out.String())
	})

	t.Run("JSON Success emits status envelope", func(t *testing.T) {
		out := &bytes.Buffer{}
		p := NewPresenter(out, OutputJSON, &bytes.Buffer{}, nil)
		p.Success("done")
		assertJSONEq(t, `{"message":"done","status":"ok"}`, out.String())
	})

	t.Run("human Error writes to stderr", func(t *testing.T) {
		out := &bytes.Buffer{}
		errOut := &bytes.Buffer{}
		p := NewPresenter(out, OutputHuman, errOut, nil)
		p.Error(errors.New("nope"))
		if errOut.Len() == 0 {
			t.Fatal("want error on stderr")
		}
		if out.Len() != 0 {
			t.Fatalf("want nothing on stdout, got %q", out.String())
		}
	})

	t.Run("JSON Error emits structured envelope", func(t *testing.T) {
		out := &bytes.Buffer{}
		p := NewPresenter(out, OutputJSON, &bytes.Buffer{}, nil)
		p.Error(errors.New("nope"))
		assertJSONEq(t, `{"error":"nope"}`, out.String())
	})

	t.Run("explicit human renderer is used", func(t *testing.T) {
		out := &bytes.Buffer{}
		human := func(w io.Writer, v any) error {
			_, _ = w.Write([]byte("custom:" + v.(LoginResult).User))
			return nil
		}
		p := NewPresenter(out, OutputHuman, &bytes.Buffer{}, human)
		p.Result(LoginResult{User: "z"})
		if out.String() != "custom:z" {
			t.Fatalf("want custom:z, got %q", out.String())
		}
	})
}

// --- tiny local test helpers ---

func assertNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("want %q in %q", want, got)
	}
}

func assertJSONEq(t *testing.T, want, got string) {
	t.Helper()
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Fatalf("want %q, got %q", want, strings.TrimSpace(got))
	}
}
