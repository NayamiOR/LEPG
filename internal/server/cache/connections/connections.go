package connections

import (
	"LEPG/internal/utils"
)

type Connection struct {
	DeviceHash    string
	ConnectionID  string
	ServerNode    string
	ClientIP      string
	ConnectedAt   utils.Timestamp
	LastHeartbeat utils.Timestamp
}

type ConnectionManager interface {
	RegisterConnection(conn *Connection) error
	UpdateHeartbeat(connection_id string) error
	GetConnection(device_hash string) (*Connection, error)
	RemoveConnection(device_hash string) error
	ListConnections() ([]*Connection, error)
	GetDeviceConnection(device_hash string) (*Connection, error)
}
