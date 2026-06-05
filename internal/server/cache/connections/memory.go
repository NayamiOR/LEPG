package connections

import (
	"fmt"
	"sync"

	"LEPG/internal/utils"
)

type MemoryConnectionManager struct {
	mu   sync.RWMutex
	conns map[string]*Connection
}

func NewMemoryConnectionManager() *MemoryConnectionManager {
	return &MemoryConnectionManager{
		conns: make(map[string]*Connection),
	}
}

func (m *MemoryConnectionManager) RegisterConnection(conn *Connection) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conns[conn.DeviceHash] = conn
	return nil
}

func (m *MemoryConnectionManager) UpdateHeartbeat(device_hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	conn, ok := m.conns[device_hash]
	if !ok {
		return fmt.Errorf("update heartbeat: connection %s not found", device_hash)
	}
	conn.LastHeartbeat = utils.NewTimestamp()
	return nil
}

func (m *MemoryConnectionManager) GetConnection(device_hash string) (*Connection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.conns[device_hash]
	if !ok {
		return nil, nil
	}
	return conn, nil
}

func (m *MemoryConnectionManager) RemoveConnection(device_hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.conns, device_hash)
	return nil
}

func (m *MemoryConnectionManager) ListConnections() ([]*Connection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Connection, 0, len(m.conns))
	for _, conn := range m.conns {
		result = append(result, conn)
	}
	return result, nil
}

func (m *MemoryConnectionManager) GetDeviceConnection(device_hash string) (*Connection, error) {
	return m.GetConnection(device_hash)
}
