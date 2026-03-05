package server

import (
	"LEPG/internal/config"
	"fmt"
	"log/slog"
	"net"
)

// ReceiveLoop 接收循环
func ReceiveLoop() error {
	cfg, err := config.GetServerConfig()
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		return err
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		// TODO: handle connection
		go HandleConnection(conn)
	}
}

func HandleConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			slog.Info("connection closed", "error", err)
			return
		}

		if n > 0 {
			slog.Info("received data", "data", string(buf[:n]), "size", n)
		}
	}
}
