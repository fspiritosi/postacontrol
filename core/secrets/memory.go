package secrets

import "sync"

// NewMemory devuelve un Store en memoria, pensado para tests. Es seguro para
// uso concurrente.
func NewMemory() Store {
	return &memoryStore{items: map[string]string{}}
}

type memoryStore struct {
	mu    sync.Mutex
	items map[string]string
}

func (m *memoryStore) Set(email, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[email] = password
	return nil
}

func (m *memoryStore) Get(email string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pw, ok := m.items[email]
	if !ok {
		return "", ErrNotFound
	}
	return pw, nil
}

func (m *memoryStore) Delete(email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, email)
	return nil
}
