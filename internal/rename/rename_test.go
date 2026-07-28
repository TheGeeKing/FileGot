package rename

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestApplyAndUndo(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "old.mkv")
	to := filepath.Join(root, "new.mkv")
	if err := os.WriteFile(from, []byte("movie"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(filepath.Join(root, "config", "last-rename.json"))
	if err := manager.Apply([]Operation{{From: from, To: to}}); err != nil {
		t.Fatal(err)
	}
	if !exists(to) || exists(from) || !manager.HasUndo() {
		t.Fatal("rename was not applied with durable undo")
	}
	if err := manager.Undo(); err != nil {
		t.Fatal(err)
	}
	if !exists(from) || exists(to) || manager.HasUndo() {
		t.Fatal("undo did not restore the source")
	}
}

func TestTransformedApplyAndUndoRestoresOriginalBytes(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "old.mkv")
	to := filepath.Join(root, "new.mkv")
	if err := os.WriteFile(from, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(root, "history.json"))
	err := manager.Apply([]Operation{{
		From: from, To: to,
		Transform: func(_, output string) error {
			return os.WriteFile(output, []byte("tagged"), 0o644)
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(to)
	if string(data) != "tagged" {
		t.Fatalf("applied content = %q", data)
	}
	if err := manager.Undo(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(from)
	if string(data) != "original" || exists(to) {
		t.Fatalf("undo content = %q", data)
	}
}

func TestTransformCanWriteMetadataWithoutRenaming(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "same.mkv")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(root, "history.json"))
	if err := manager.Apply([]Operation{{
		From: path, To: path,
		Transform: func(_, output string) error {
			return os.WriteFile(output, []byte("tagged"), 0o644)
		},
	}}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "tagged" {
		t.Fatalf("written content = %q", data)
	}
	if err := manager.Undo(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != "original" {
		t.Fatalf("undo content = %q", data)
	}
}

func TestTransformFailureLeavesOriginalIntact(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "old.mkv")
	to := filepath.Join(root, "new.mkv")
	if err := os.WriteFile(from, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(root, "history.json"))
	err := manager.Apply([]Operation{{
		From: from, To: to,
		Transform: func(_, output string) error {
			_ = os.WriteFile(output, []byte("partial"), 0o644)
			return errors.New("failed")
		},
	}})
	if err == nil {
		t.Fatal("transform failure should be returned")
	}
	data, _ := os.ReadFile(from)
	if string(data) != "original" || exists(to) {
		t.Fatalf("failed transform changed files: %q", data)
	}
}

func TestTransformedUndoFailureReturnsToAppliedState(t *testing.T) {
	root := t.TempDir()
	var operations []Operation
	for _, name := range []string{"one", "two"} {
		from := filepath.Join(root, name+".mkv")
		to := filepath.Join(root, name+"-new.mkv")
		if err := os.WriteFile(from, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		operations = append(operations, Operation{
			From: from, To: to,
			Transform: func(_, output string) error {
				return os.WriteFile(output, []byte("tagged"), 0o644)
			},
		})
	}
	manager := NewManager(filepath.Join(root, "history.json"))
	if err := manager.Apply(operations); err != nil {
		t.Fatal(err)
	}
	failed := false
	manager.rename = func(old, new string) error {
		if !failed && new == operations[1].From {
			failed = true
			return errors.New("blocked")
		}
		return os.Rename(old, new)
	}
	if err := manager.Undo(); err == nil {
		t.Fatal("undo should fail")
	}
	for _, operation := range operations {
		if !exists(operation.To) || exists(operation.From) {
			t.Fatal("failed undo did not restore the applied state")
		}
	}
	if !manager.HasUndo() {
		t.Fatal("failed undo should remain retryable")
	}
}

func TestRecoveryKeepsTaggedFileWhenOriginalBackupIsMissing(t *testing.T) {
	root := t.TempDir()
	to := filepath.Join(root, "new.mkv")
	if err := os.WriteFile(to, []byte("tagged"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(root, "history.json"))
	if err := manager.writeJournal(journal{
		Version: journalVersion, State: "pending", Mode: "apply",
		Ops: []journalOperation{{
			From: filepath.Join(root, "old.mkv"),
			Temp: filepath.Join(root, "missing.mkv"),
			To:   to, Tagged: filepath.Join(root, "tagged.tmp.mkv"),
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Recover(); err == nil {
		t.Fatal("recovery should report the missing original backup")
	}
	if !exists(to) {
		t.Fatal("recovery deleted the only surviving media copy")
	}
}

func TestSwap(t *testing.T) {
	root := t.TempDir()
	left := filepath.Join(root, "left.mkv")
	right := filepath.Join(root, "right.mkv")
	if err := os.WriteFile(left, []byte("left"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(right, []byte("right"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(filepath.Join(root, "history.json"))
	if err := manager.Apply([]Operation{{From: left, To: right}, {From: right, To: left}}); err != nil {
		t.Fatal(err)
	}
	leftData, _ := os.ReadFile(left)
	rightData, _ := os.ReadFile(right)
	if string(leftData) != "right" || string(rightData) != "left" {
		t.Fatalf("swap result left=%q right=%q", leftData, rightData)
	}
}

func TestCollisionRefused(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "old.mkv")
	to := filepath.Join(root, "existing.mkv")
	for _, path := range []string{from, to} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	manager := NewManager(filepath.Join(root, "history.json"))
	if err := manager.Apply([]Operation{{From: from, To: to}}); err == nil {
		t.Fatal("existing destination should be refused")
	}
	if !exists(from) || !exists(to) {
		t.Fatal("collision check changed files")
	}
}

func TestNoChangeRefused(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "same.mkv")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(root, "history.json"))
	if err := manager.Apply([]Operation{{From: path, To: path}}); err == nil {
		t.Fatal("no-op batch should be refused")
	}
}

func TestDuplicateSourceRefused(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "same.mkv")
	if err := os.WriteFile(from, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(root, "history.json"))
	err := manager.Apply([]Operation{
		{From: from, To: filepath.Join(root, "one.mkv")},
		{From: from, To: filepath.Join(root, "two.mkv")},
	})
	if err == nil {
		t.Fatal("duplicate source should be refused")
	}
	if !exists(from) {
		t.Fatal("duplicate source check changed the file")
	}
}

func TestRollbackOnFailure(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.mkv")
	second := filepath.Join(root, "second.mkv")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	manager := NewManager(filepath.Join(root, "history.json"))
	realRename := manager.rename
	var calls int
	manager.rename = func(from, to string) error {
		calls++
		if calls == 4 {
			return errors.New("forced failure")
		}
		return realRename(from, to)
	}
	err := manager.Apply([]Operation{
		{From: first, To: filepath.Join(root, "first-new.mkv")},
		{From: second, To: filepath.Join(root, "second-new.mkv")},
	})
	if err == nil {
		t.Fatal("forced failure should be returned")
	}
	if !exists(first) || !exists(second) {
		t.Fatal("rollback did not restore all sources")
	}
}

func TestRecoverPendingApply(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "old.mkv")
	temp := filepath.Join(root, ".filegot-temp.tmp")
	to := filepath.Join(root, "new.mkv")
	if err := os.WriteFile(temp, []byte("movie"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(filepath.Join(root, "history.json"))
	record := journal{
		Version: journalVersion, State: "pending", Mode: "apply",
		Ops: []journalOperation{{From: from, Temp: temp, To: to}},
	}
	if err := manager.writeJournal(record); err != nil {
		t.Fatal(err)
	}
	if err := manager.Recover(); err != nil {
		t.Fatal(err)
	}
	if !exists(from) || exists(temp) || manager.HasPendingRecovery() {
		t.Fatal("pending operation was not recovered")
	}
}
