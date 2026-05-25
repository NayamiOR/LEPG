#!/usr/bin/env python3
"""
LEPG Modbus 测试模拟器
使用 pymodbus 模拟真实传感器设备，为网关提供测试数据源
"""

from pymodbus.server.sync import StartTcpServer
from pymodbus.datastore import ModbusServerContext, ModbusSlaveContext
from pymodbus.datastore.store import ModbusSequentialDataBlock
import struct
import threading


def create_temp_sensor():
    """温湿度传感器 - TCP 5020, slave_id=1

    寄存器定义:
      HR[0]: 温度 250 → 25.0°C (scale=0.1)
      HR[1]: 湿度 650 → 65.0% (scale=0.1)
      IR[0-9]: 输入寄存器测试数据 [100, 110, 120, ...]
    """
    hr_block = ModbusSequentialDataBlock(1, [250, 650] + [0] * 98)
    ir_block = ModbusSequentialDataBlock(1, [100, 110, 120, 130, 140, 150, 160, 170, 180, 190] + [0] * 90)

    return ModbusSlaveContext(hr=hr_block, ir=ir_block)


def create_power_meter():
    """智能电表 - TCP 5021, slave_id=1

    寄存器定义:
      HR[100]: 电压 2200 → 220.0V (scale=0.1)
      HR[102-103]: 电流 10.00A (float32, IEEE 754 big-endian)
      HR[104-105]: 功率 2.2kW (float32)
    """
    block = ModbusSequentialDataBlock(0, [0] * 200)

    # 电压
    block.setValues(101, [2200])

    # 电流 10.00A → float32 bytes → uint16 words
    current_float = struct.pack('>f', 10.0)
    current_words = [struct.unpack('>H', current_float[i:i+2])[0] for i in range(0, 4, 2)]
    block.setValues(103, current_words)

    # 功率 2.2kW
    power_float = struct.pack('>f', 2.2)
    power_words = [struct.unpack('>H', power_float[i:i+2])[0] for i in range(0, 4, 2)]
    block.setValues(105, power_words)

    return ModbusSlaveContext(hr=block)


def create_plc_controller():
    """PLC控制器 - TCP 5022, slave_id=1

    寄存器定义:
      CO[0-9]: 运行状态、开关状态 (bool, 交替: 1,0,1,0...)
      DI[0-7]: 输入状态 (bool)
    """
    coil_block = ModbusSequentialDataBlock(0, [1, 0, 1, 0, 1, 0, 1, 0, 1, 0])
    di_block = ModbusSequentialDataBlock(0, [1, 1, 0, 1, 0, 0, 1, 1])
    return ModbusSlaveContext(di=di_block, co=coil_block)


def create_multi_register():
    """多寄存器设备 - TCP 5023, slave_id=2

    寄存器定义:
      HR[200-201]: 计数器 1000 (int32)
      HR[300-301]: 设定值 5000 (uint32)
      HR[400]: 单寄存器值 999 (uint16)
    """
    block = ModbusSequentialDataBlock(0, [0] * 500)

    # int32: 1000 → [1000, 0] (big-endian, 高字在前)
    block.setValues(200, [1000, 0])

    # uint32: 5000
    block.setValues(300, [5000, 0])

    # uint16 测试 FC6/FC16 写操作
    block.setValues(400, [999])

    return ModbusSlaveContext(hr=block)


def start_device(port, slave_context, slave_id=1):
    """启动单个 TCP 设备"""
    context = ModbusServerContext(slaves={slave_id: slave_context}, single=False)
    StartTcpServer(context=context, address=("127.0.0.1", port))


def main():
    """启动所有测试设备"""
    print("=" * 60)
    print("LEPG Modbus 测试模拟器")
    print("=" * 60)

    devices = [
        (5020, create_temp_sensor(), 1, "温湿度传感器"),
        (5021, create_power_meter(), 1, "智能电表"),
        (5022, create_plc_controller(), 1, "PLC控制器"),
        (5023, create_multi_register(), 2, "多寄存器设备"),
    ]

    threads = []
    for port, ctx, sid, name in devices:
        t = threading.Thread(target=start_device, args=(port, ctx, sid))
        t.daemon = True
        t.start()
        threads.append(t)
        print(f"+ {name:12s} -> 127.0.0.1:{port} (slave_id={sid})")

    print()
    print("设备功能覆盖:")
    print("  - FC1/2: 读写线圈/离散输入 (bool)")
    print("  - FC3/4: 读写保持/输入寄存器 (int16/uint16)")
    print("  - FC6/16: 写单/多寄存器 (int32/uint32/float32)")
    print()
    print("按 Ctrl+C 停止所有设备")
    print("=" * 60)
    print()

    try:
        for t in threads:
            t.join()
    except KeyboardInterrupt:
        print("\n所有设备已停止")


if __name__ == "__main__":
    main()
