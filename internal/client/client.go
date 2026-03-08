package client

import (
	"LEPG/internal/config"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

func MainFunc() error {
	wg := &sync.WaitGroup{}
	wg.Go(func() {
		if err := TestWrite(); err != nil {
			slog.Error("TestWrite failed", "err", err)
		}
	})
	wg.Go(func() {
		if err := UploadLoop(); err != nil {
			slog.Error("UploadLoop failed", "err", err)
		}
	})

	wg.Wait()
	return nil
}

func TestWrite() error {
	x := 0

	cfg, err := config.GetClientConfig()
	if err != nil {
		return err
	}
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", cfg.ServerUrl, cfg.Port))
	if err != nil {
		return err
	}

	for {
		fmt.Println(x)
		_, err := fmt.Fprintf(conn, "hello %d", x)
		if err != nil {
			return err
		}
		time.Sleep(time.Nanosecond * 100)
		x += 1
	}

}

func UploadLoop() error {
	for {
		time.Sleep(time.Millisecond * 500)

	}
}
