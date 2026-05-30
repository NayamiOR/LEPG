package client

import (
	"LEPG/internal/client/cache"
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

func MainFunc(ctx context.Context, cfg *ClientConfig) error {
	slog.Info("Client configuration", "config", cfg)

	if len(cfg.Devices) == 0 {
		slog.Warn("No devices configured, client will run without Modbus polling")
		return nil
	}

	store, err := cache.NewSQLiteStore(ctx, cfg.Paths.DataPath)
	if err != nil {
		return fmt.Errorf("create SQLite store: %w", err)
	}
	defer store.Close()

	ch := make(chan cache.Reading, cfg.BufferSize)
	var mainWg sync.WaitGroup

	// Goroutine 1: 轮询 → channel
	mainWg.Go(func() {
		slog.Info("Starting device polling")
		var pollWg sync.WaitGroup
		for _, device := range cfg.Devices {
			slog.Info("Starting:", "device", device.Name, "type", device.Type)
			pollWg.Go(func() {
				if err := TcpDevicePolling(ch, device); err != nil {
					slog.Error("Device polling failed", "device", device.Name, "error", err)
				}
			})
		}
		pollWg.Wait()
		close(ch)
	})

	// Goroutine 2: channel → SQLite
	mainWg.Go(func() {
		batchSize := 10
		maxInterval := 30 * time.Second
		consumeAndWrite(ctx, ch, store, batchSize, maxInterval)
	})

	// Goroutine 3: SQLite → 上传（占位）
	mainWg.Go(func() {
		uploadLoop(ctx, cfg, store)
	})

	mainWg.Wait()
	return nil
}

func consumeAndWrite(ctx context.Context, ch <-chan cache.Reading, store cache.Store, batchSize int, maxInterval time.Duration) {
	buffer := make([]*cache.Reading, 0, batchSize)
	ticker := time.NewTicker(maxInterval)
	defer ticker.Stop()

	flush := func() {
		if len(buffer) == 0 {
			return
		}
		if err := store.SaveReadings(ctx, buffer); err != nil {
			slog.Error("Failed to save readings", "error", err)
			return
		}
		slog.Info("Saved readings")
		buffer = buffer[:0]
	}

	for {
		select {
		case r, ok := <-ch:
			if !ok {
				flush()
				return
			}
			buffer = append(buffer, &r)
			if len(buffer) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}

func uploadLoop(ctx context.Context, cfg *ClientConfig, store cache.Store) {
	slog.Info("Upload loop started")
	<-ctx.Done()
	slog.Info("Upload loop stopped")
}
