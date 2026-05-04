package proxy

import (
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// DomainListState represents the runtime state of a domain list.
type DomainListState struct {
	Name        string    `yaml:"name"`
	DomainCount int       `yaml:"domain_count"`
	LastUpdated time.Time `yaml:"last_updated"`
}

// StateManager manages the runtime state of domain lists.
type StateManager struct {
	stateFile string
	states    map[string]*DomainListState
	mu        sync.RWMutex
}

// NewStateManager creates a new state manager.
func NewStateManager(stateFile string) *StateManager {
	return &StateManager{
		stateFile: stateFile,
		states:    make(map[string]*DomainListState),
	}
}

// LoadState loads state from file.
func (sm *StateManager) LoadState() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	data, err := os.ReadFile(sm.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// State file doesn't exist yet, that's ok
			return nil
		}
		return fmt.Errorf("read state file: %w", err)
	}

	var states []DomainListState
	if err := yaml.Unmarshal(data, &states); err != nil {
		return fmt.Errorf("unmarshal state: %w", err)
	}

	for i := range states {
		sm.states[states[i].Name] = &states[i]
	}

	return nil
}

// SaveState saves state to file.
func (sm *StateManager) SaveState() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	states := make([]DomainListState, 0, len(sm.states))
	for _, state := range sm.states {
		states = append(states, *state)
	}

	data, err := yaml.Marshal(states)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(sm.stateFile, data, 0644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}

// UpdateState updates the state for a domain list.
func (sm *StateManager) UpdateState(name string, domainCount int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.states[name] = &DomainListState{
		Name:        name,
		DomainCount: domainCount,
		LastUpdated: time.Now(),
	}
}

// GetState returns the state for a domain list.
func (sm *StateManager) GetState(name string) *DomainListState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.states[name]
}

// GetAllStates returns all states.
func (sm *StateManager) GetAllStates() map[string]*DomainListState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[string]*DomainListState, len(sm.states))
	for k, v := range sm.states {
		result[k] = v
	}

	return result
}
