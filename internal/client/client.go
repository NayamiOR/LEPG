package client

import (
	"LEPG/internal/config"
	"fmt"
	"net"
	"time"
)

func MainFunc() error {
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
		_, err := conn.Write([]byte(fmt.Sprintf("hello %d", x)))
		if err != nil {
			return err
		}
		time.Sleep(time.Second * 2)
		x += 1
	}
}

func UploadLoop() error {
	for {
		time.Sleep(time.Millisecond * 500)

	}
}
