package jj_test

import (
	"strings"
	"testing"

	"github.com/madalinpopa/jjreview/internal/jj"
)

func operations(t *testing.T, client jj.Client) []jj.Operation {
	t.Helper()
	ops, err := client.Operations(t.Context())
	if err != nil {
		t.Fatalf("Operations() error = %v", err)
	}
	return ops
}

func operationRepo(t *testing.T) (string, jj.Client) {
	t.Helper()
	dir := newRepo(t)
	writeFile(t, dir, "a.txt", "a\n")
	runJJ(t, dir, "describe", "-m", "first")
	runJJ(t, dir, "describe", "-m", "second")
	return dir, newClient(t, dir)
}

func TestOperations_success(t *testing.T) {
	t.Run("lists the operations newest first", func(t *testing.T) {
		_, client := operationRepo(t)

		ops := operations(t, client)

		if len(ops) < 3 {
			t.Fatalf("len(Operations()) = %d, want at least 3", len(ops))
		}
		head := ops[0]
		if !head.Current || head.Root || head.Snapshot {
			t.Errorf("ops[0] = %+v, want the current, non-root, non-snapshot operation", head)
		}
		if !strings.HasPrefix(head.Description, "describe commit") {
			t.Errorf("ops[0].Description = %q, want a describe operation", head.Description)
		}
		if head.Args != "jj describe -m second" {
			t.Errorf("ops[0].Args = %q, want %q", head.Args, "jj describe -m second")
		}
		if head.ID == "" || head.Time == "" || head.User == "" {
			t.Errorf("ops[0] = %+v, want an ID, a time and a user", head)
		}
		root := ops[len(ops)-1]
		if !root.Root || root.Current {
			t.Errorf("last op = %+v, want the root operation", root)
		}
		for _, op := range ops[1:] {
			if op.Current {
				t.Errorf("op %s Current = true, want only the head to be current", op.ID)
			}
		}
	})

	t.Run("marks snapshot operations", func(t *testing.T) {
		_, client := operationRepo(t)

		ops := operations(t, client)

		found := false
		for _, op := range ops {
			if op.Snapshot {
				found = true
				if op.Description != "snapshot working copy" {
					t.Errorf("snapshot op Description = %q, want %q", op.Description, "snapshot working copy")
				}
			}
		}
		if !found {
			t.Errorf("Operations() = %+v, want a snapshot operation", ops)
		}
	})
}

func TestOperations_successArgsWithoutClientFlags(t *testing.T) {
	_, client := operationRepo(t)
	head := operations(t, client)[0]
	if err := client.RevertOperation(t.Context(), head.ID); err != nil {
		t.Fatalf("RevertOperation(%q) error = %v", head.ID, err)
	}

	got := operations(t, client)[0].Args

	if want := "jj operation revert -- " + head.ID; got != want {
		t.Errorf("Args = %q, want %q", got, want)
	}
}

func TestOperations_failed(t *testing.T) {
	if _, err := newClient(t, t.TempDir()).Operations(t.Context()); err == nil {
		t.Error("Operations() outside a repo error = nil, want an error")
	}
}

func TestRestoreOperation_success(t *testing.T) {
	_, client := operationRepo(t)
	ops := operations(t, client)
	first := ops[1]

	if err := client.RestoreOperation(t.Context(), first.ID); err != nil {
		t.Fatalf("RestoreOperation(%q) error = %v", first.ID, err)
	}

	if got := loadChange(t, client, "@").Description; got != "first\n" {
		t.Errorf("@ description = %q, want %q", got, "first\n")
	}
	if after := operations(t, client); after[0].ID == ops[0].ID || after[1].ID != ops[0].ID {
		t.Errorf("head after restore = %s, want a new operation on top of %s", after[0].ID, ops[0].ID)
	}
}

func TestRevertOperation_success(t *testing.T) {
	_, client := operationRepo(t)
	ops := operations(t, client)

	if err := client.RevertOperation(t.Context(), ops[0].ID); err != nil {
		t.Fatalf("RevertOperation(%q) error = %v", ops[0].ID, err)
	}

	if got := loadChange(t, client, "@").Description; got != "first\n" {
		t.Errorf("@ description = %q, want %q", got, "first\n")
	}
}

func TestRevertOperation_failed(t *testing.T) {
	_, client := operationRepo(t)
	ops := operations(t, client)
	root := ops[len(ops)-1]

	if err := client.RevertOperation(t.Context(), root.ID); err == nil {
		t.Error("RevertOperation(root) error = nil, want an error")
	}
}

func TestAbandonOperation_success(t *testing.T) {
	_, client := operationRepo(t)
	ops := operations(t, client)
	first := ops[1]

	if err := client.AbandonOperation(t.Context(), first.ID); err != nil {
		t.Fatalf("AbandonOperation(%q) error = %v", first.ID, err)
	}

	after := operations(t, client)
	for _, op := range after {
		if op.ID == first.ID {
			t.Errorf("Operations() still lists the abandoned %s", first.ID)
		}
	}
	if got := loadChange(t, client, "@").Description; got != "second\n" {
		t.Errorf("@ description = %q, want %q", got, "second\n")
	}
}

func TestAbandonOperation_failed(t *testing.T) {
	_, client := operationRepo(t)
	ops := operations(t, client)

	if err := client.AbandonOperation(t.Context(), ops[0].ID); err == nil {
		t.Error("AbandonOperation(current) error = nil, want an error")
	}
}
