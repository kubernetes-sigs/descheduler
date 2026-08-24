/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cycle

import (
	"errors"
	"sync"
)

var (
	ErrAlreadyRunning = errors.New("descheduling cycle already running")
	ErrRunnerNotReady = errors.New("descheduling cycle runner is not ready")
)

type Runner func() error

type Manager struct {
	mu      sync.Mutex
	running bool
	runner  Runner
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) SetRunner(runner Runner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runner = runner
}

func (m *Manager) Run() error {
	runner, err := m.start()
	if err != nil {
		return err
	}
	defer m.finish()
	return runner()
}

func (m *Manager) TryStart() error {
	runner, err := m.start()
	if err != nil {
		return err
	}
	go func() {
		defer m.finish()
		_ = runner()
	}()
	return nil
}

func (m *Manager) start() (Runner, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runner == nil {
		return nil, ErrRunnerNotReady
	}
	if m.running {
		return nil, ErrAlreadyRunning
	}
	m.running = true
	return m.runner, nil
}

func (m *Manager) finish() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
}
