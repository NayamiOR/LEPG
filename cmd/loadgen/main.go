// loadgen 是 LEPG 服务端的协议层压测工具。
//
// 它模拟 N 个独立 client（各自 SN/Token，独立 TCP 连接），
// 每个 client 走完整握手鉴权后，以指定批量大小持续上传 TLV Upload 帧，
// 统计服务端吞吐（readings/s）、帧级 RTT 分位数（P50/P95/P99）与错误率。
//
// 用法示例：
//
//	# 50 client 并发压测 30s，每帧 100 readings
//	go run ./cmd/loadgen -server 124.223.200.168:8883 -clients 50 -duration 30s -batch 100
//
// 前置条件：服务端 server.toml 需预注册对应的 [[clients]]（SN 见 -sn-prefix）。
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"LEPG/internal/model"
	"LEPG/internal/msg"
)

var (
	server         = flag.String("server", "127.0.0.1:8883", "server address host:port")
	clients        = flag.Int("clients", 10, "number of concurrent client connections")
	duration       = flag.Duration("duration", 30*time.Second, "load duration")
	batch          = flag.Int("batch", 100, "readings per Upload frame")
	snPrefix       = flag.String("sn-prefix", "LOADGEN", "client SN prefix (SN = prefix-001..N)")
	token          = flag.String("token", "token123456", "client token")
	devPrefix      = flag.String("dev-prefix", "dev", "device name prefix (device = prefix-<sn尾号>, 跨压测端用不同前缀隔离)")
	reportInterval = flag.Duration("report-interval", 0, "print per-window throughput every interval (0 = disabled)")
	idle           = flag.Bool("idle", false, "hold connection after handshake without sending data (connection-capacity test)")
	verbose        = flag.Bool("v", false, "verbose per-client logging")

	totalSent atomic.Int64
)

type clientResult struct {
	sn        string
	sent      int64 // readings acked（只在 ACK OK 后累加；throughput 用的是此口径，非发出量）
	acked     int64 // frames acked OK
	failed    int64 // frames failed (non-OK ack / io error)
	latencies []time.Duration
}

func main() {
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	if *batch <= 0 || *clients <= 0 {
		fmt.Fprintln(os.Stderr, "clients/batch must be positive")
		os.Exit(1)
	}

	fmt.Printf("== loadgen start ==\n  server=%s clients=%d duration=%s batch=%d\n",
		*server, *clients, *duration, *batch)

	if *reportInterval > 0 {
		go reportLoop(*duration)
	}

	start := time.Now()
	var wg sync.WaitGroup
	results := make([]*clientResult, *clients)

	for i := 0; i < *clients; i++ {
		wg.Add(1)
		res := &clientResult{sn: fmt.Sprintf("%s-%03d", *snPrefix, i+1)}
		results[i] = res
		go func(r *clientResult) {
			defer wg.Done()
			runClient(r)
		}(res)
	}

	wg.Wait()
	elapsed := time.Since(start)

	// 汇总
	var totalSent, totalAcked, totalFailed int64
	allLat := []time.Duration{}
	for _, r := range results {
		totalSent += r.sent
		totalAcked += r.acked
		totalFailed += r.failed
		allLat = append(allLat, r.latencies...)
	}

	tps := float64(totalSent) / elapsed.Seconds()
	frameCount := int64(len(allLat))
	fmt.Printf("\n== loadgen done ==\n")
	fmt.Printf("  elapsed:        %s\n", elapsed.Round(time.Millisecond))
	if *idle {
		fmt.Printf("  connections established: %d\n", totalAcked)
		fmt.Printf("  connections failed:      %d\n", totalFailed)
	} else {
		fmt.Printf("  readings sent:  %d\n", totalSent)
		fmt.Printf("  frames acked:   %d\n", totalAcked)
		fmt.Printf("  frames failed:  %d\n", totalFailed)
		fmt.Printf("  throughput:     %.0f readings/s\n", tps)
	}
	if frameCount > 0 {
		sort.Slice(allLat, func(i, j int) bool { return allLat[i] < allLat[j] })
		p := func(q float64) time.Duration {
			return allLat[int(float64(frameCount)*q)]
		}
		avg := int64(0)
		for _, l := range allLat {
			avg += int64(l)
		}
		avg /= frameCount
		fmt.Printf("  frame RTT:      avg=%s p50=%s p90=%s p95=%s p99=%s max=%s\n",
			time.Duration(avg).Round(time.Microsecond),
			p(0.50).Round(time.Microsecond),
			p(0.90).Round(time.Microsecond),
			p(0.95).Round(time.Microsecond),
			p(0.99).Round(time.Microsecond),
			allLat[frameCount-1].Round(time.Microsecond))
	}
}

// reportLoop 每 reportInterval 打印一次窗口内已确认读数与瞬时吞吐（用于 QPS 曲线）。
func reportLoop(total time.Duration) {
	ticker := time.NewTicker(*reportInterval)
	defer ticker.Stop()
	start := time.Now()
	lastN := int64(0)
	lastT := start
	for t := range ticker.C {
		cur := totalSent.Load()
		dt := t.Sub(lastT).Seconds()
		if dt > 0 {
			fmt.Printf("[t=%s] +%d readings (%.0f/s) total=%d\n",
				t.Sub(start).Round(time.Second), cur-lastN, float64(cur-lastN)/dt, cur)
		}
		lastN = cur
		lastT = t
	}
	_ = total
}

func runClient(r *clientResult) {
	conn, err := net.DialTimeout("tcp", *server, 5*time.Second)
	if err != nil {
		slog.Error("dial failed", "sn", r.sn, "err", err)
		return
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(*duration + 10*time.Second))

	factory := msg.NewMsgFactory()

	// 1. 握手
	hs, err := factory.NewMsg(msg.MsgTypeHandshake, &msg.HandshakePayload{
		Sn:              r.sn,
		Token:           *token,
		FirmwareVersion: 1,
	})
	if err != nil {
		slog.Error("handshake encode", "sn", r.sn, "err", err)
		return
	}
	frame, err := hs.Encode()
	if err != nil {
		slog.Error("handshake encode", "sn", r.sn, "err", err)
		return
	}
	if _, err := conn.Write(frame); err != nil {
		slog.Error("handshake write", "sn", r.sn, "err", err)
		return
	}
	ack, err := readAck(conn, factory, msg.MsgTypeHandshakeAck)
	// 注意：readAck 出错时返回 nil ack，必须先判 err 再取 ack.Code，
	// 否则压测中任何一次握手失败都会 nil 解引用 panic —— 而汇总输出在
	// wg.Wait() 之后，进程崩溃会连带丢掉已采集的全部数据。
	if err != nil {
		r.failed++
		slog.Error("handshake read failed", "sn", r.sn, "err", err)
		return
	}
	if ack.Code != msg.Ok {
		r.failed++
		slog.Error("handshake rejected", "sn", r.sn, "code", ack.Code)
		return
	}
	if *verbose {
		slog.Info("authenticated", "sn", r.sn)
	}

	// 2a. 保活模式（连接承载测试）：握手成功后不发数据，仅定期发心跳保持连接存活
	if *idle {
		r.acked++ // 连接建立成功
		hbDeadline := time.Now().Add(*duration)
		lastHb := time.Now()
		for time.Now().Before(hbDeadline) {
			if time.Since(lastHb) >= 30*time.Second {
				m, err := factory.NewMsg(msg.MsgTypeHeartbeat, &msg.HeartbeatPayload{})
				if err != nil {
					return
				}
				frame, err := m.Encode()
				if err != nil {
					return
				}
				if _, err := conn.Write(frame); err != nil {
					r.failed++
					return
				}
				lastHb = time.Now()
			}
			time.Sleep(1 * time.Second)
		}
		return
	}

	// 2b. 持续上传
	deadline := time.Now().Add(*duration)
	for time.Now().Before(deadline) {
		payload := &msg.UploadPayload{Readings: buildReadings(r.sn, *batch)}
		m, err := factory.NewMsg(msg.MsgTypeUpload, payload)
		if err != nil {
			slog.Error("upload encode", "sn", r.sn, "err", err)
			return
		}
		frame, err := m.Encode()
		if err != nil {
			slog.Error("upload encode", "sn", r.sn, "err", err)
			return
		}
		t0 := time.Now()
		if _, err := conn.Write(frame); err != nil {
			r.failed++
			slog.Error("upload write", "sn", r.sn, "err", err)
			return
		}
		ack, err := readAck(conn, factory, msg.MsgTypeUploadAck)
		// 同样先判 err：失败时 ack 为 nil，且此时 t0 起的等待往往是
		// 读取超时（可达 conn deadline 级别），计入 RTT 会把 P95/P99
		// 严重拉高 —— 因此只在读到合法 ACK 后才记录延迟。
		if err != nil {
			r.failed++
			if *verbose {
				slog.Warn("upload ack read failed", "sn", r.sn, "err", err)
			}
			continue
		}
		r.latencies = append(r.latencies, time.Since(t0))
		if ack.Code != msg.Ok {
			r.failed++
			if *verbose {
				slog.Warn("upload rejected", "sn", r.sn, "code", ack.Code)
			}
			continue
		}
		r.acked++
		r.sent += int64(*batch)
		totalSent.Add(int64(*batch))
	}
}

// tsSeq 保证所有 client 生成的 reading timestamp 全局唯一（对账/去重语义干净）。
var tsSeq atomic.Int64

// buildReadings 构造 batch 条合法的 readings。
// device 名按 SN 尾号区分（每个 client 模拟独立设备），timestamp 全局唯一，
// 保证 (device, point, timestamp) 跨 client/跨进程不撞键，对账与去重语义干净。
func buildReadings(sn string, n int) []model.Reading {
	rs := make([]model.Reading, 0, n)
	base := time.Now().UnixMilli()
	dev := *devPrefix + "-" + sn[len(sn)-3:] // dev-001..010
	for i := 0; i < n; i++ {
		ts := tsSeq.Add(1)
		rs = append(rs, model.Reading{
			ID:         base*1000 + ts,
			Device:     dev,
			DeviceName: dev,
			Point:      "pt-000001",
			PointName:  "pt-000001",
			DataType:   model.DataTypeUint16,
			Value:      "42",
			Quality:    model.QualityGood,
			Unit:       "",
			Timestamp:  ts,
		})
	}
	return rs
}

// ackResult 是服务端 ACK 帧的解析结果。
type ackResult struct {
	MsgID uint16
	Code  uint8
}

func readAck(conn net.Conn, factory *msg.MsgFactory, expectType uint8) (*ackResult, error) {
	hdr := make([]byte, msg.HeaderSize)
	if _, err := readFull(conn, hdr); err != nil {
		return nil, err
	}
	// 简单解析 header：Magic(2) Ver(1) Flags(1) Type(1) MsgID(2) PayloadLen(2) Timestamp(4)
	if hdr[0] != byte(msg.MagicNumber>>8) || hdr[1] != byte(msg.MagicNumber&0xff) {
		return nil, fmt.Errorf("bad magic")
	}
	msgType := hdr[4]
	payloadLen := binary.BigEndian.Uint16(hdr[7:9])
	rest := make([]byte, payloadLen+2) // payload + checksum
	if _, err := readFull(conn, rest); err != nil {
		return nil, err
	}
	_ = factory
	// 校验帧类型：否则任何非 ACK 帧（心跳响应、Push 等）都会被按 ACK 载荷
	// 布局解析，Code 位置恰好命中 msg.Ok 时就会虚假判成功，低估错误率。
	if msgType != expectType {
		return nil, fmt.Errorf("unexpected frame type: got %d, want %d", msgType, expectType)
	}
	// AckPayload wire: [MsgID:2B BE][Code:1B]
	if payloadLen < 3 {
		return nil, fmt.Errorf("ack payload too short: %d", payloadLen)
	}
	return &ackResult{
		MsgID: binary.BigEndian.Uint16(rest[0:2]),
		Code:  rest[2],
	}, nil
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}
