package rename

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const journalVersion = 1

type Operation struct {
	From string
	To   string
}

type journalOperation struct {
	From string `json:"from"`
	Temp string `json:"temp"`
	To   string `json:"to"`
}

type journal struct {
	Version   int                `json:"version"`
	State     string             `json:"state"`
	Mode      string             `json:"mode"`
	CreatedAt time.Time          `json:"created_at"`
	Ops       []journalOperation `json:"operations"`
}

type Manager struct {
	journalPath string
	rename      func(string, string) error
}

func NewManager(journalPath string) *Manager {
	return &Manager{journalPath: journalPath, rename: os.Rename}
}

func (manager *Manager) HasUndo() bool {
	record, err := manager.readJournal()
	return err == nil && record.State == "applied" && record.Mode == "apply"
}

func (manager *Manager) HasPendingRecovery() bool {
	record, err := manager.readJournal()
	return err == nil && record.State == "pending"
}

func (manager *Manager) Apply(operations []Operation) error {
	record, err := manager.prepare(operations, "apply")
	if err != nil {
		return err
	}
	if len(record.Ops) == 0 {
		return errors.New("no filename changes to apply")
	}
	if err := manager.writeJournal(record); err != nil {
		return fmt.Errorf("write rename journal: %w", err)
	}
	if err := manager.execute(record); err != nil {
		return err
	}
	record.State = "applied"
	if err := manager.writeJournal(record); err != nil {
		rollbackErr := manager.rollback(record)
		if rollbackErr == nil {
			_ = os.Remove(manager.journalPath)
		}
		return errors.Join(fmt.Errorf("durable undo could not be finalized; rename was reverted: %w", err), rollbackErr)
	}
	return nil
}

func (manager *Manager) Undo() error {
	applied, err := manager.readJournal()
	if err != nil {
		return fmt.Errorf("load rename history: %w", err)
	}
	if applied.State != "applied" || applied.Mode != "apply" {
		return errors.New("there is no completed rename to undo")
	}

	operations := make([]Operation, 0, len(applied.Ops))
	for _, operation := range applied.Ops {
		operations = append(operations, Operation{From: operation.To, To: operation.From})
	}
	record, err := manager.prepare(operations, "undo")
	if err != nil {
		return err
	}
	if err := manager.writeJournal(record); err != nil {
		return fmt.Errorf("write undo journal: %w", err)
	}
	if err := manager.execute(record); err != nil {
		return err
	}
	if err := os.Remove(manager.journalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("undo succeeded but history could not be cleared: %w", err)
	}
	return nil
}

func (manager *Manager) Recover() error {
	record, err := manager.readJournal()
	if err != nil {
		return fmt.Errorf("load pending rename: %w", err)
	}
	if record.State != "pending" {
		return errors.New("there is no interrupted rename to recover")
	}
	if err := manager.rollback(record); err != nil {
		return err
	}

	if record.Mode == "undo" {
		for index := range record.Ops {
			record.Ops[index].From, record.Ops[index].To = record.Ops[index].To, record.Ops[index].From
			record.Ops[index].Temp = ""
		}
		record.State = "applied"
		record.Mode = "apply"
		return manager.writeJournal(record)
	}
	return os.Remove(manager.journalPath)
}

func (manager *Manager) prepare(operations []Operation, mode string) (journal, error) {
	record := journal{
		Version: journalVersion, State: "pending", Mode: mode, CreatedAt: time.Now().UTC(),
	}
	if err := preflight(operations); err != nil {
		return record, err
	}
	for _, operation := range operations {
		from, _ := filepath.Abs(operation.From)
		to, _ := filepath.Abs(operation.To)
		if samePath(from, to) {
			continue
		}
		temp, err := temporaryPath(filepath.Dir(from))
		if err != nil {
			return record, err
		}
		record.Ops = append(record.Ops, journalOperation{From: from, Temp: temp, To: to})
	}
	return record, nil
}

func preflight(operations []Operation) error {
	sources := make(map[string]string, len(operations))
	destinations := make(map[string]string, len(operations))

	for _, operation := range operations {
		from, err := filepath.Abs(operation.From)
		if err != nil {
			return err
		}
		to, err := filepath.Abs(operation.To)
		if err != nil {
			return err
		}
		if !samePath(filepath.Dir(from), filepath.Dir(to)) {
			return fmt.Errorf("destination must stay in the source directory: %s", to)
		}
		info, err := os.Stat(from)
		if err != nil {
			return fmt.Errorf("source %s: %w", from, err)
		}
		if info.IsDir() {
			return fmt.Errorf("source is a directory: %s", from)
		}

		sourceKey := pathKey(from)
		if previous, exists := sources[sourceKey]; exists {
			return fmt.Errorf("duplicate source paths: %s and %s", previous, from)
		}
		sources[sourceKey] = from

		destinationKey := pathKey(to)
		if previous, exists := destinations[destinationKey]; exists {
			return fmt.Errorf("duplicate destination paths: %s and %s", previous, to)
		}
		destinations[destinationKey] = to
	}

	for key, destination := range destinations {
		if _, source := sources[key]; source {
			continue
		}
		if _, err := os.Stat(destination); err == nil {
			return fmt.Errorf("destination already exists: %s", destination)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("check destination %s: %w", destination, err)
		}
	}
	return nil
}

func (manager *Manager) execute(record journal) error {
	for index, operation := range record.Ops {
		if err := manager.rename(operation.From, operation.Temp); err != nil {
			rollbackErr := manager.rollback(record)
			return errors.Join(fmt.Errorf("stage %s: %w", operation.From, err), rollbackErr)
		}
		record.Ops[index] = operation
	}
	for _, operation := range record.Ops {
		if err := manager.rename(operation.Temp, operation.To); err != nil {
			rollbackErr := manager.rollback(record)
			return errors.Join(fmt.Errorf("rename %s: %w", operation.From, err), rollbackErr)
		}
	}
	return nil
}

func (manager *Manager) rollback(record journal) error {
	var rollbackErrors []error
	for index := len(record.Ops) - 1; index >= 0; index-- {
		operation := record.Ops[index]
		switch {
		case exists(operation.To) && !exists(operation.From):
			if err := manager.rename(operation.To, operation.From); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("%s -> %s: %w", operation.To, operation.From, err))
			}
		case exists(operation.Temp) && !exists(operation.From):
			if err := manager.rename(operation.Temp, operation.From); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("%s -> %s: %w", operation.Temp, operation.From, err))
			}
		}
	}
	if len(rollbackErrors) > 0 {
		return fmt.Errorf("rollback incomplete: %w", errors.Join(rollbackErrors...))
	}
	return nil
}

func (manager *Manager) readJournal() (journal, error) {
	var record journal
	data, err := os.ReadFile(manager.journalPath)
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, fmt.Errorf("decode rename journal: %w", err)
	}
	if record.Version != journalVersion {
		return record, fmt.Errorf("unsupported rename journal version %d", record.Version)
	}
	return record, nil
}

func (manager *Manager) writeJournal(record journal) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(manager.journalPath), 0o700); err != nil {
		return err
	}
	temp := manager.journalPath + ".tmp"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	if err := os.Remove(manager.journalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temp, manager.journalPath)
}

func temporaryPath(directory string) (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return filepath.Join(directory, ".filegot-"+hex.EncodeToString(random[:])+".tmp"), nil
}

func pathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

func samePath(left, right string) bool {
	return pathKey(left) == pathKey(right)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
