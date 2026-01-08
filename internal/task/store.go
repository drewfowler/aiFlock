package task

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	defaultConfigDir = ".flock"
	tasksFile        = "tasks.json"
)

// Store handles task persistence to JSON files
type Store struct {
	path string
}

// NewStore creates a new store at the default location (~/.flock/tasks.json)
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configDir := filepath.Join(home, defaultConfigDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	return &Store{
		path: filepath.Join(configDir, tasksFile),
	}, nil
}

// NewStoreWithPath creates a new store at the specified path
func NewStoreWithPath(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Store{path: path}, nil
}

// Load loads tasks from the JSON file
func (s *Store) Load() ([]*Task, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Task{}, nil
		}
		return nil, err
	}

	var tasks []*Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, err
	}

	return tasks, nil
}

// Save saves tasks to the JSON file
func (s *Store) Save(tasks []*Task) error {
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0644)
}

// Path returns the store file path
func (s *Store) Path() string {
	return s.path
}
