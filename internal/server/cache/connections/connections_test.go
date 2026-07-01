package connections

import (
	"LEPG/internal/utils"
	"testing"
)

func TestMemoryConnectionManager_RegisterAndGet(t *testing.T) {
	mgr := NewMemoryConnectionManager()

	conn := &Connection{
		DeviceHash:    "CLIENT001",
		ConnectionID:  "conn-123",
		ClientIP:      "192.168.1.100:12345",
		ConnectedAt:   utils.NewTimestamp(),
		LastHeartbeat: utils.NewTimestamp(),
	}

	if err := mgr.RegisterConnection(conn); err != nil {
		t.Fatalf("RegisterConnection: %v", err)
	}

	got, err := mgr.GetConnection("CLIENT001")
	if err != nil {
		t.Fatalf("GetConnection: %v", err)
	}
	if got == nil {
		t.Fatal("expected connection, got nil")
	}
	if got.ConnectionID != "conn-123" {
		t.Errorf("ConnectionID: got %q, want conn-123", got.ConnectionID)
	}
	if got.ClientIP != "192.168.1.100:12345" {
		t.Errorf("ClientIP: got %q", got.ClientIP)
	}
}

func TestMemoryConnectionManager_GetDeviceConnection(t *testing.T) {
	mgr := NewMemoryConnectionManager()
	conn := &Connection{DeviceHash: "DEV01", ConnectionID: "c1"}
	mgr.RegisterConnection(conn)

	got, err := mgr.GetDeviceConnection("DEV01")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConnectionID != "c1" {
		t.Errorf("expected c1, got %q", got.ConnectionID)
	}
}

func TestMemoryConnectionManager_UpdateHeartbeat(t *testing.T) {
	mgr := NewMemoryConnectionManager()
	oldTs := utils.Timestamp(1000)
	conn := &Connection{
		DeviceHash:    "CLIENT001",
		LastHeartbeat: oldTs,
	}
	mgr.RegisterConnection(conn)

	if err := mgr.UpdateHeartbeat("CLIENT001"); err != nil {
		t.Fatalf("UpdateHeartbeat: %v", err)
	}

	updated, _ := mgr.GetConnection("CLIENT001")
	// 心跳时间戳 >= 原值（毫秒级可能相同，不应回退）
	if updated.LastHeartbeat < oldTs {
		t.Errorf("heartbeat should not decrease: was %d, now %d", oldTs, updated.LastHeartbeat)
	}
}

func TestMemoryConnectionManager_UpdateHeartbeat_NotFound(t *testing.T) {
	mgr := NewMemoryConnectionManager()
	err := mgr.UpdateHeartbeat("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent device")
	}
}

func TestMemoryConnectionManager_RemoveConnection(t *testing.T) {
	mgr := NewMemoryConnectionManager()
	mgr.RegisterConnection(&Connection{DeviceHash: "DEV01"})
	mgr.RegisterConnection(&Connection{DeviceHash: "DEV02"})

	if err := mgr.RemoveConnection("DEV01"); err != nil {
		t.Fatalf("RemoveConnection: %v", err)
	}

	got, _ := mgr.GetConnection("DEV01")
	if got != nil {
		t.Error("DEV01 should be removed")
	}

	got, _ = mgr.GetConnection("DEV02")
	if got == nil {
		t.Error("DEV02 should still exist")
	}
}

func TestMemoryConnectionManager_ListConnections(t *testing.T) {
	mgr := NewMemoryConnectionManager()
	mgr.RegisterConnection(&Connection{DeviceHash: "A"})
	mgr.RegisterConnection(&Connection{DeviceHash: "B"})
	mgr.RegisterConnection(&Connection{DeviceHash: "C"})

	list, err := mgr.ListConnections()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 connections, got %d", len(list))
	}

	hashes := make(map[string]bool)
	for _, c := range list {
		hashes[c.DeviceHash] = true
	}
	for _, h := range []string{"A", "B", "C"} {
		if !hashes[h] {
			t.Errorf("missing DeviceHash %q in list", h)
		}
	}
}

func TestMemoryConnectionManager_RegisterOverwrite(t *testing.T) {
	mgr := NewMemoryConnectionManager()
	mgr.RegisterConnection(&Connection{DeviceHash: "DEV01", ConnectionID: "old"})
	mgr.RegisterConnection(&Connection{DeviceHash: "DEV01", ConnectionID: "new"})

	got, _ := mgr.GetConnection("DEV01")
	if got.ConnectionID != "new" {
		t.Errorf("expected 'new' after overwrite, got %q", got.ConnectionID)
	}
}

func TestMemoryConnectionManager_GetNotFound(t *testing.T) {
	mgr := NewMemoryConnectionManager()
	got, err := mgr.GetConnection("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent, got %+v", got)
	}
}
