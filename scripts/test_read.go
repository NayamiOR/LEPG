package main

import (
	"log/slog"
	"time"

	"github.com/goburrow/modbus"
)

func main() {
	slog.Info("Starting Modbus TCP Example")
	// read localhost:5020, slave_id 1
	handler := modbus.NewTCPClientHandler("127.0.0.1:5020")
	handler2 := modbus.NewTCPClientHandler("127.0.0.1:5021")
	handler.SlaveId = 1
	handler.Timeout = 5 * time.Second
	handler2.SlaveId = 1
	handler2.Timeout = 5 * time.Second
	client := modbus.NewClient(handler)
	client2 := modbus.NewClient(handler2)
	err := handler.Connect()
	if err != nil {
		slog.Error("Error connecting to Modbus TCP server", "error", err)
		return

	}
	slog.Info("Connected to Modbus TCP server")
	defer handler.Close()
	err = handler2.Connect()
	if err != nil {
		slog.Error("Error connecting to Modbus TCP server", "error", err)
		return
	}
	slog.Info("Connected to Modbus TCP server")
	defer handler2.Close()

	pollInterval := 1 * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for range ticker.C {
		t, err := client.ReadHoldingRegisters(0, 1)
		if err != nil {
			slog.Error("Error reading holding registers", "error", err)
			return
		}
		h, err := client.ReadHoldingRegisters(1, 1)
		if err != nil {
			slog.Error("Error reading holding registers", "error", err)
			return
		}
		inp, err := client.ReadInputRegisters(0, 10)
		if err != nil {
			slog.Error("Error reading input registers", "error", err)
			return
		}

		slog.Info("Read holding registers", "t", t, "h", h)
		slog.Info("Read input registers", "inp", inp)
		v, err := client2.ReadHoldingRegisters(100, 1)
		if err != nil {
			slog.Error("Error reading holding registers", "error", err)
			return
		}
		i, err := client2.ReadHoldingRegisters(102, 2)
		if err != nil {
			slog.Error("Error reading input registers", "error", err)
			return
		}
		slog.Info("Read holding registers", "v", v)
		slog.Info("Read input registers", "i", i)
		p, err := client2.ReadHoldingRegisters(104, 2)
		if err != nil {
			slog.Error("Error reading coils", "error", err)
			return
		}
		slog.Info("Read coils", "p", p)
	}
}
