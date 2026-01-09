package task

import (
	"fmt"
	"sync"
)

// Manager handles task CRUD operations
type Manager struct {
	tasks   map[string]*Task
	order   []string // maintains insertion order
	store   *Store
	mu      sync.RWMutex
	counter int
}

// NewManager creates a new task manager with the given store
func NewManager(store *Store) *Manager {
	return &Manager{
		tasks: make(map[string]*Task),
		order: make([]string, 0),
		store: store,
	}
}

// Load loads tasks from the store
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tasks, err := m.store.Load()
	if err != nil {
		return err
	}

	m.tasks = make(map[string]*Task)
	m.order = make([]string, 0, len(tasks))

	for _, t := range tasks {
		m.tasks[t.ID] = t
		m.order = append(m.order, t.ID)
		// Update counter to be higher than any existing ID
		var id int
		if _, err := fmt.Sscanf(t.ID, "%d", &id); err == nil && id >= m.counter {
			m.counter = id + 1
		}
	}

	return nil
}

// Save persists tasks to the store
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]*Task, 0, len(m.order))
	for _, id := range m.order {
		tasks = append(tasks, m.tasks[id])
	}
	return m.store.Save(tasks)
}

// Create creates a new task
func (m *Manager) Create(name, promptFile, cwd string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("%03d", m.counter)
	m.counter++

	task := NewTask(id, name, promptFile, cwd)
	m.tasks[id] = task
	m.order = append(m.order, id)

	// Save after creation
	tasks := make([]*Task, 0, len(m.order))
	for _, oid := range m.order {
		tasks = append(tasks, m.tasks[oid])
	}
	if err := m.store.Save(tasks); err != nil {
		return nil, err
	}

	return task, nil
}

// NextID returns the next task ID that will be assigned (without incrementing)
func (m *Manager) NextID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return fmt.Sprintf("%03d", m.counter)
}

// Get returns a task by ID
func (m *Manager) Get(id string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task, ok := m.tasks[id]
	return task, ok
}

// Update updates a task's fields
func (m *Manager) Update(id string, fn func(*Task)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}

	fn(task)

	// Save after update
	tasks := make([]*Task, 0, len(m.order))
	for _, oid := range m.order {
		tasks = append(tasks, m.tasks[oid])
	}
	return m.store.Save(tasks)
}

// UpdateStatus updates a task's status
func (m *Manager) UpdateStatus(id string, status Status) error {
	return m.Update(id, func(t *Task) {
		t.Status = status
	})
}

// Delete removes a task by ID
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tasks[id]; !ok {
		return fmt.Errorf("task %s not found", id)
	}

	delete(m.tasks, id)

	// Remove from order
	newOrder := make([]string, 0, len(m.order)-1)
	for _, oid := range m.order {
		if oid != id {
			newOrder = append(newOrder, oid)
		}
	}
	m.order = newOrder

	// Save after deletion
	tasks := make([]*Task, 0, len(m.order))
	for _, oid := range m.order {
		tasks = append(tasks, m.tasks[oid])
	}
	return m.store.Save(tasks)
}

// List returns all tasks in order
func (m *Manager) List() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]*Task, 0, len(m.order))
	for _, id := range m.order {
		tasks = append(tasks, m.tasks[id])
	}
	return tasks
}

// FindByTabName finds a task by its tab name
func (m *Manager) FindByTabName(tabName string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, task := range m.tasks {
		if task.TabName == tabName {
			return task, true
		}
	}
	return nil, false
}

// Count returns the number of tasks
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tasks)
}

// ActiveCount returns the number of active tasks
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, task := range m.tasks {
		if task.IsActive() {
			count++
		}
	}
	return count
}

// WaitingCount returns the number of tasks waiting for input
func (m *Manager) WaitingCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, task := range m.tasks {
		if task.NeedsAttention() {
			count++
		}
	}
	return count
}
