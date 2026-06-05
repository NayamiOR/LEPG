package connections

import (
	"context"
	"fmt"
	"strconv"

	"LEPG/internal/utils"

	"github.com/redis/go-redis/v9"
)

const (
	connKeyPrefix = "lepg:device:"
)

type RedisConnectionManager struct {
	client *redis.Client
}

func NewRedisConnectionManager(client *redis.Client) *RedisConnectionManager {
	return &RedisConnectionManager{client: client}
}

func (m *RedisConnectionManager) RegisterConnection(conn *Connection) error {
	ctx := context.Background()
	key := connKeyPrefix + conn.DeviceHash
	_, err := m.client.HSet(ctx, key, map[string]string{
		"device_hash":    conn.DeviceHash,
		"connection_id":  conn.ConnectionID,
		"server_node":    conn.ServerNode,
		"client_ip":      conn.ClientIP,
		"connected_at":   strconv.FormatUint(uint64(conn.ConnectedAt), 10),
		"last_heartbeat": strconv.FormatUint(uint64(conn.LastHeartbeat), 10),
	}).Result()
	if err != nil {
		return fmt.Errorf("register connection %s: %w", conn.DeviceHash, err)
	}
	return nil
}

func (m *RedisConnectionManager) UpdateHeartbeat(device_hash string) error {
	ctx := context.Background()
	key := connKeyPrefix + device_hash

	pipe := m.client.Pipeline()
	exists := pipe.Exists(ctx, key)
	pipe.HSet(ctx, key, "last_heartbeat", strconv.FormatUint(uint64(utils.NewTimestamp()), 10))
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("update heartbeat for %s: %w", device_hash, err)
	}
	if exists.Val() == 0 {
		return fmt.Errorf("update heartbeat: connection %s not found", device_hash)
	}
	return nil
}

func (m *RedisConnectionManager) GetConnection(device_hash string) (*Connection, error) {
	ctx := context.Background()
	result, err := m.client.HGetAll(ctx, connKeyPrefix+device_hash).Result()
	if err != nil {
		return nil, fmt.Errorf("get connection %s: %w", device_hash, err)
	}
	if len(result) == 0 {
		return nil, nil
	}
	return connectionFromHash(result)
}

func (m *RedisConnectionManager) RemoveConnection(device_hash string) error {
	ctx := context.Background()
	_, err := m.client.Del(ctx, connKeyPrefix+device_hash).Result()
	if err != nil {
		return fmt.Errorf("remove connection %s: %w", device_hash, err)
	}
	return nil
}

func (m *RedisConnectionManager) ListConnections() ([]*Connection, error) {
	ctx := context.Background()
	keys, err := m.client.Keys(ctx, connKeyPrefix+"*").Result()
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	if len(keys) == 0 {
		return []*Connection{}, nil
	}

	pipe := m.client.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.HGetAll(ctx, key)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}

	conns := make([]*Connection, 0, len(keys))
	for _, cmd := range cmds {
		result := cmd.Val()
		if len(result) == 0 {
			continue
		}
		conn, err := connectionFromHash(result)
		if err != nil {
			return nil, err
		}
		conns = append(conns, conn)
	}
	return conns, nil
}

func (m *RedisConnectionManager) GetDeviceConnection(device_hash string) (*Connection, error) {
	return m.GetConnection(device_hash)
}

func connectionFromHash(m map[string]string) (*Connection, error) {
	connectedAt, err := strconv.ParseUint(m["connected_at"], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid connected_at: %w", err)
	}
	lastHeartbeat, err := strconv.ParseUint(m["last_heartbeat"], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid last_heartbeat: %w", err)
	}
	return &Connection{
		DeviceHash:    m["device_hash"],
		ConnectionID:  m["connection_id"],
		ServerNode:    m["server_node"],
		ClientIP:      m["client_ip"],
		ConnectedAt:   utils.Timestamp(connectedAt),
		LastHeartbeat: utils.Timestamp(lastHeartbeat),
	}, nil
}
