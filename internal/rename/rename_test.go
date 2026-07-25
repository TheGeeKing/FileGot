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
