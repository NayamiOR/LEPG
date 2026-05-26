package client

import (
	"LEPG/internal/model"
	"testing"
)

func tcpDevice(host string, port int, slaveID byte) *DeviceConfig {
	return &DeviceConfig{
		Type:    model.ConnectionTypeTCP,
		TCP:     &TcpSlaveConfig{Host: host, Port: port},
		SlaveID: slaveID,
	}
}

func rtuDevice(port string, slaveID byte) *DeviceConfig {
	return &DeviceConfig{
		Type:    model.ConnectionTypeRTU,
		RTU:     &RtuSlaveConfig{Port: port},
		SlaveID: slaveID,
	}
}

func TestDeviceHash_Length(t *testing.T) {
	d := tcpDevice("192.168.1.1", 502, 1)
	h, err := d.Hash()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("TCP 192.168.1.1:502 slaveID=1 → %s", h)
	if len(h) != 16 {
		t.Fatalf("expected 16-char hash, got %d: %s", len(h), h)
	}
}

func TestDeviceHash_Deterministic(t *testing.T) {
	d := tcpDevice("192.168.1.1", 502, 1)
	h1, _ := d.Hash()
	h2, _ := d.Hash()
	if h1 != h2 {
		t.Fatalf("same device produced different hashes: %s vs %s", h1, h2)
	}
}

func TestDeviceHash_TCP_DifferentDevices(t *testing.T) {
	d1 := tcpDevice("192.168.1.1", 502, 1)
	d2 := tcpDevice("192.168.1.2", 502, 1)
	d3 := tcpDevice("192.168.1.1", 503, 1)
	d4 := tcpDevice("192.168.1.1", 502, 2)
	h1, _ := d1.Hash()
	h2, _ := d2.Hash()
	h3, _ := d3.Hash()
	h4, _ := d4.Hash()
	t.Logf("TCP d1: %s → %s", "192.168.1.1:502/1", h1)
	t.Logf("TCP d2: %s → %s", "192.168.1.2:502/1", h2)
	t.Logf("TCP d3: %s → %s", "192.168.1.1:503/1", h3)
	t.Logf("TCP d4: %s → %s", "192.168.1.1:502/2", h4)
	if h1 == h2 || h1 == h3 || h1 == h4 {
		t.Fatal("different TCP devices produced same hash")
	}
}

func TestDeviceHash_RTU_DifferentDevices(t *testing.T) {
	d1 := rtuDevice("/dev/ttyS", 1)
	d2 := rtuDevice("/dev/ttyS0", 1)
	d3 := rtuDevice("/dev/ttyS0", 2)
	h1, _ := d1.Hash()
	h2, _ := d2.Hash()
	h3, _ := d3.Hash()
	t.Logf("RTU d1: %s → %s", "/dev/ttyS/1", h1)
	t.Logf("RTU d2: %s → %s", "/dev/ttyS0/1", h2)
	t.Logf("RTU d3: %s → %s", "/dev/ttyS0/2", h3)
	if h1 == h2 || h1 == h3 {
		t.Fatal("different RTU devices produced same hash")
	}
}

func TestDeviceHash_CrossType(t *testing.T) {
	dr := rtuDevice("COM3", 1)
	dt := tcpDevice("COM3", 1, 1)
	h1, _ := dr.Hash()
	h2, _ := dt.Hash()
	t.Logf("RTU COM3/1 → %s", h1)
	t.Logf("TCP COM3:1/1 → %s", h2)
	if h1 == h2 {
		t.Fatal("RTU and TCP with same port/slaveID produced same hash")
	}
}

func TestDeviceHash_Errors(t *testing.T) {
	cases := []struct {
		name string
		d    *DeviceConfig
	}{
		{"nil TCP", &DeviceConfig{Type: model.ConnectionTypeTCP}},
		{"nil RTU", &DeviceConfig{Type: model.ConnectionTypeRTU}},
		{"unknown type", &DeviceConfig{Type: "unknown"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.d.Hash()
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestDeviceHash_GoldenValue(t *testing.T) {
	// Ensure hash stability across runs
	h, err := tcpDevice("192.168.1.100", 502, 1).Hash()
	if err != nil {
		t.Fatal(err)
	}
	// Pre-computed: SHA256("tcp:192.168.1.100:502:1")[:8] as hex
	expected := "80a79d1566db91f0"
	if h != expected {
		t.Fatalf("golden value mismatch: got %s, expected %s (input was %q)", h, expected, "tcp:192.168.1.100:502:1")
	}
}
