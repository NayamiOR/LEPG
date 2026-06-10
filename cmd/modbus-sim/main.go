// LEPG Modbus 测试模拟器
// 模拟四个工业传感器设备，为网关提供测试数据源
//
// 手写最小 Modbus TCP server，支持 FC1/2/3/4/6/16，使用 PDU 0-based 寻址。
package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"os/signal"
	"syscall"
)

// Modbus function codes
const (
	fcReadCoils              = 0x01
	fcReadDiscreteInputs     = 0x02
	fcReadHoldingRegisters   = 0x03
	fcReadInputRegisters     = 0x04
	fcWriteSingleRegister    = 0x06
	fcWriteMultipleRegisters = 0x10
)

// deviceStore 存储单个 Modbus 设备的寄存器/线圈数据
type deviceStore struct {
	name    string
	slaveID uint8
	hr      map[uint16]uint16 // holding registers
	ir      map[uint16]uint16 // input registers
	coils   map[uint16]bool   // coils (FC1)
	di      map[uint16]bool   // discrete inputs (FC2)
}

func newDeviceStore(name string, slaveID uint8) *deviceStore {
	return &deviceStore{
		name:    name,
		slaveID: slaveID,
		hr:      make(map[uint16]uint16),
		ir:      make(map[uint16]uint16),
		coils:   make(map[uint16]bool),
		di:      make(map[uint16]bool),
	}
}

func float32ToWords(f float32) (uint16, uint16) {
	bits := math.Float32bits(f)
	return uint16(bits >> 16), uint16(bits)
}

// serve 在指定地址上启动 Modbus TCP server，监听到 ctx 取消时退出
func (s *deviceStore) serve(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go s.handleConn(conn)
	}
}

// handleConn 处理单个 TCP 连接上的 Modbus 请求
func (s *deviceStore) handleConn(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)

	for {
		// MBAP header: TxID(2) + ProtoID(2) + Length(2) + UnitID(1) = 7 bytes
		var hdr [7]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return
		}

		txID := binary.BigEndian.Uint16(hdr[0:2])
		length := binary.BigEndian.Uint16(hdr[4:6])
		unitID := hdr[6]

		// PDU 长度 = Length - 1 (UnitID)
		pduLen := int(length) - 1
		if pduLen <= 0 {
			return
		}
		pdu := make([]byte, pduLen)
		if _, err := io.ReadFull(r, pdu); err != nil {
			return
		}

		if unitID != s.slaveID {
			continue
		}

		respPDU := s.processPDU(pdu)
		if respPDU == nil {
			return
		}

		// MBAP response: TxID(2) + ProtoID(2) + Length(2) + UnitID(1) + PDU
		respLen := uint16(1 + len(respPDU))
		resp := make([]byte, 6+1+len(respPDU))
		binary.BigEndian.PutUint16(resp[0:], txID)
		binary.BigEndian.PutUint16(resp[4:], respLen)
		resp[6] = unitID
		copy(resp[7:], respPDU)

		if _, err := conn.Write(resp); err != nil {
			return
		}
	}
}

func (s *deviceStore) processPDU(pdu []byte) []byte {
	if len(pdu) == 0 {
		return nil
	}
	switch pdu[0] {
	case fcReadHoldingRegisters:
		return s.readRegs(s.hr, pdu)
	case fcReadInputRegisters:
		return s.readRegs(s.ir, pdu)
	case fcReadCoils:
		return s.readBits(s.coils, pdu)
	case fcReadDiscreteInputs:
		return s.readBits(s.di, pdu)
	case fcWriteSingleRegister:
		return s.writeSingleReg(pdu)
	case fcWriteMultipleRegisters:
		return s.writeMultiReg(pdu)
	default:
		return exceptResp(pdu[0], 0x01)
	}
}

// readRegs 处理 FC3/FC4
func (s *deviceStore) readRegs(data map[uint16]uint16, pdu []byte) []byte {
	fc := pdu[0]
	if len(pdu) < 5 {
		return exceptResp(fc, 0x03)
	}
	addr := binary.BigEndian.Uint16(pdu[1:3])
	qty := binary.BigEndian.Uint16(pdu[3:5])
	if qty < 1 || qty > 125 {
		return exceptResp(fc, 0x03)
	}

	resp := make([]byte, 2+int(qty)*2)
	resp[0] = fc
	resp[1] = byte(qty * 2)
	for i := range qty {
		v, ok := data[addr+i]
		if !ok {
			return exceptResp(fc, 0x02)
		}
		binary.BigEndian.PutUint16(resp[2+i*2:], v)
	}
	return resp
}

// readBits 处理 FC1/FC2
func (s *deviceStore) readBits(data map[uint16]bool, pdu []byte) []byte {
	fc := pdu[0]
	if len(pdu) < 5 {
		return exceptResp(fc, 0x03)
	}
	addr := binary.BigEndian.Uint16(pdu[1:3])
	qty := binary.BigEndian.Uint16(pdu[3:5])
	if qty < 1 || qty > 2000 {
		return exceptResp(fc, 0x03)
	}

	byteCount := byte((qty + 7) / 8)
	resp := make([]byte, 2+int(byteCount))
	resp[0] = fc
	resp[1] = byteCount
	for i := range qty {
		v, ok := data[addr+i]
		if !ok {
			return exceptResp(fc, 0x02)
		}
		if v {
			resp[2+i/8] |= 1 << (i % 8)
		}
	}
	return resp
}

// writeSingleReg 处理 FC6
func (s *deviceStore) writeSingleReg(pdu []byte) []byte {
	if len(pdu) < 5 {
		return exceptResp(fcWriteSingleRegister, 0x03)
	}
	addr := binary.BigEndian.Uint16(pdu[1:3])
	val := binary.BigEndian.Uint16(pdu[3:5])
	s.hr[addr] = val
	return pdu[:5] // echo
}

// writeMultiReg 处理 FC16
func (s *deviceStore) writeMultiReg(pdu []byte) []byte {
	if len(pdu) < 6 {
		return exceptResp(fcWriteMultipleRegisters, 0x03)
	}
	addr := binary.BigEndian.Uint16(pdu[1:3])
	qty := binary.BigEndian.Uint16(pdu[3:5])
	bc := int(pdu[5])

	if bc != int(qty)*2 || len(pdu) < 6+bc {
		return exceptResp(fcWriteMultipleRegisters, 0x03)
	}
	for i := range qty {
		s.hr[addr+i] = binary.BigEndian.Uint16(pdu[6+i*2:])
	}

	resp := make([]byte, 5)
	resp[0] = fcWriteMultipleRegisters
	binary.BigEndian.PutUint16(resp[1:3], addr)
	binary.BigEndian.PutUint16(resp[3:5], qty)
	return resp
}

func exceptResp(fc, code byte) []byte {
	return []byte{fc | 0x80, code}
}

// --- 设备数据创建 ---

// 温湿度传感器 - TCP 5020, slave_id=1
// HR[0]=250 (25.0°C, scale=0.1), HR[1]=650 (65.0%, scale=0.1)
// IR[0..9]=[100,110,...,190]
func createTempSensor() *deviceStore {
	s := newDeviceStore("温湿度传感器", 1)
	s.hr[0] = 250
	s.hr[1] = 650
	for i := range uint16(10) {
		s.ir[i] = 100 + i*10
	}
	return s
}

// 智能电表 - TCP 5021, slave_id=1
// HR[100]=2200 (220.0V, scale=0.1)
// HR[102..103]=float32(10.0) 电流
// HR[104..105]=float32(2.2)  功率
func createPowerMeter() *deviceStore {
	s := newDeviceStore("智能电表", 1)
	s.hr[100] = 2200
	hi, lo := float32ToWords(10.0)
	s.hr[102] = hi
	s.hr[103] = lo
	hi, lo = float32ToWords(2.2)
	s.hr[104] = hi
	s.hr[105] = lo
	return s
}

// PLC控制器 - TCP 5022, slave_id=1
// CO[0..9]=[1,0,1,0,...]  DI[0..7]=[1,1,0,1,0,0,1,1]
func createPLCController() *deviceStore {
	s := newDeviceStore("PLC控制器", 1)
	for i := range uint16(10) {
		s.coils[i] = i%2 == 0
	}
	s.di[0] = true
	s.di[1] = true
	s.di[3] = true
	s.di[6] = true
	s.di[7] = true
	return s
}

// 多寄存器设备 - TCP 5023, slave_id=2
// HR[199..200]=int32(1000)  HR[299..300]=uint32(5000)  HR[399]=uint16(999)
func createMultiRegister() *deviceStore {
	s := newDeviceStore("多寄存器设备", 2)
	// int32 1000 big-endian words: [0, 1000]
	s.hr[199] = 0
	s.hr[200] = 1000
	// uint32 5000 big-endian words: [0, 5000]
	s.hr[299] = 0
	s.hr[300] = 5000
	// uint16
	s.hr[399] = 999
	return s
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	devices := []struct {
		store *deviceStore
		port  int
	}{
		{createTempSensor(), 5020},
		{createPowerMeter(), 5021},
		{createPLCController(), 5022},
		{createMultiRegister(), 5023},
	}

	fmt.Println("============================================================")
	fmt.Println("LEPG Modbus 测试模拟器")
	fmt.Println("============================================================")

	for _, d := range devices {
		addr := fmt.Sprintf("127.0.0.1:%d", d.port)
		go func() {
			if err := d.store.serve(ctx, addr); err != nil {
				slog.Error("设备启动失败", "name", d.store.name, "error", err)
			}
		}()
		fmt.Printf("+ %-14s -> %s (slave_id=%d)\n", d.store.name, addr, d.store.slaveID)
	}

	fmt.Println()
	fmt.Println("设备功能覆盖:")
	fmt.Println("  - FC1/2:  读写线圈/离散输入 (bool)")
	fmt.Println("  - FC3/4:  读写保持/输入寄存器 (int16/uint16)")
	fmt.Println("  - FC6/16: 写单/多寄存器 (int32/uint32/float32)")
	fmt.Println()
	fmt.Println("按 Ctrl+C 停止所有设备")
	fmt.Println("============================================================")
	fmt.Println()

	<-ctx.Done()
	fmt.Println("\n所有设备已停止")
}
