# LEPG Modbus 测试模拟器

使用 pymodbus 模拟真实传感器设备，为 LEPG 网关提供测试数据源。

## 安装

```bash
pip install -r scripts/requirements.txt
```

## 使用

```bash
python scripts/modbus_simulator.py
```

输出示例：
```
============================================================
LEPG Modbus 测试模拟器
============================================================
✓ 温湿度传感器    → 127.0.0.1:5020 (slave_id=1)
✓ 智能电表        → 127.0.0.1:5021 (slave_id=1)
✓ PLC控制器      → 127.0.0.1:5022 (slave_id=1)
✓ 多寄存器设备    → 127.0.0.1:5023 (slave_id=2)

设备功能覆盖:
  • FC1/2: 读写线圈/离散输入 (bool)
  • FC3/4: 读写保持/输入寄存器 (int16/uint16)
  • FC6/16: 写单/多寄存器 (int32/uint32/float32)

按 Ctrl+C 停止所有设备
============================================================
```

## 测试设备

### 1. 温湿度传感器 (TCP 5020, slave_id=1)

| 寄存器 | 地址 | 原始值 | scale | 工程值 | 单位 |
|--------|------|--------|-------|--------|------|
| temperature | HR[1] | 250 | 0.1 | 25.0 | °C |
| humidity | HR[2] | 650 | 0.1 | 65.0 | % |

**功能码**: 3 (读保持寄存器)
**数据类型**: int16

### 2. 智能电表 (TCP 5021, slave_id=1)

| 寄存器 | 地址 | 原始值 | scale | 工程值 | 单位 |
|--------|------|--------|-------|--------|------|
| voltage | HR[100] | 2200 | 0.1 | 220.0 | V |
| current | HR[102-103] | float32 | - | 10.00 | A |
| power | HR[104-105] | float32 | - | 2.2 | kW |

**功能码**: 3/4 (读保持/输入寄存器)
**数据类型**: uint16, float32

### 3. PLC控制器 (TCP 5022, slave_id=1)

| 寄存器 | 地址 | 值 | 说明 |
|--------|------|-----|------|
| coils | CO[0-9] | 1,0,1,0... | 开关状态 |
| discrete_inputs | DI[0-7] | 1,1,0,1... | 输入状态 |

**功能码**: 1/2/5 (读线圈/离散输入/写线圈)
**数据类型**: bool

### 4. 多寄存器设备 (TCP 5023, slave_id=2)

| 寄存器 | 地址 | 原始值 | 工程值 | 类型 |
|--------|------|--------|--------|------|
| counter | HR[200-201] | [1000,0] | 1000 | int32 |
| setpoint | HR[300-301] | [5000,0] | 5000 | uint32 |
| value | HR[400] | 999 | 999 | uint16 |

**功能码**: 3/6/16 (读/写寄存器)
**数据类型**: int32, uint32

## Client 配置示例

### 温湿度传感器

```toml
[[devices]]
name = "温湿度传感器"
type = "tcp"
[devices.tcp]
host = "127.0.0.1"
port = 5020
slave_id = 1

[[devices.points]]
name = "temperature"
function_code = 3
address = 1
quantity = 1
data_type = "int16"
scale = 0.1
unit = "°C"
access = "ro"
cache_enabled = true

[[devices.points]]
name = "humidity"
function_code = 3
address = 2
quantity = 1
data_type = "int16"
scale = 0.1
unit = "%"
access = "ro"
cache_enabled = true
```

### 智能电表

```toml
[[devices]]
name = "智能电表"
type = "tcp"
[devices.tcp]
host = "127.0.0.1"
port = 5021
slave_id = 1

[[devices.points]]
name = "voltage"
function_code = 3
address = 100
quantity = 1
data_type = "uint16"
scale = 0.1
unit = "V"

[[devices.points]]
name = "current"
function_code = 3
address = 102
quantity = 2
data_type = "float32"
unit = "A"

[[devices.points]]
name = "power"
function_code = 3
address = 104
quantity = 2
data_type = "float32"
unit = "kW"
```

### PLC控制器

```toml
[[devices]]
name = "PLC控制器"
type = "tcp"
[devices.tcp]
host = "127.0.0.1"
port = 5022
slave_id = 1

[[devices.points]]
name = "running_status"
function_code = 1
address = 0
quantity = 1
data_type = "bool"
access = "ro"

[[devices.points]]
name = "switch_control"
function_code = 5
address = 0
quantity = 1
data_type = "bool"
access = "rw"
```

### 多寄存器设备

```toml
[[devices]]
name = "多寄存器设备"
type = "tcp"
[devices.tcp]
host = "127.0.0.1"
port = 5023
slave_id = 2

[[devices.points]]
name = "counter"
function_code = 3
address = 200
quantity = 2
data_type = "int32"

[[devices.points]]
name = "setpoint"
function_code = 6
address = 300
quantity = 2
data_type = "uint32"
access = "rw"
```

## 测试流程

1. **启动模拟器**:
   ```bash
   python scripts/modbus_simulator.py
   ```

2. **配置 LEPG client** (config/client.toml):
   使用上面的配置示例

3. **运行 client**:
   ```bash
   go run cmd/client/main.go run
   ```

4. **验证数据输出**:
   检查 client 是否正确解析并输出传感器数据

## 数据编码说明

### float32 (IEEE 754 big-endian)
- 10.00A → `[0x4480, 0x0000]` → `[17560, 0]`
- 2.2kW → `[0x4004, 0x4120]` → `[17476, 16640]`

### int32/uint32 (big-endian)
- 1000 → `[0x03E8, 0x0000]` → `[1000, 0]`
- 高 16 位在低地址

## 故障排查

### "Port already in use"
```bash
# Linux/Mac
lsof -ti:5020 | xargs kill -9

# Windows
netstat -ano | findstr ":5020"
taskkill /F /PID <PID>
```

### "No module named pymodbus"
```bash
pip install pymodbus==2.5.3
```

### Client 连接失败
- 检查防火墙设置
- 确认模拟器正在运行
- 验证 host/port/slave_id 配置正确

## 注意事项

- pymodbus 2.5.3 使用 0-based 寻址
- 所有设备同时启动，占用 5020-5023 端口
- 寄存器地址在文档中已标注，使用时直接对应
- float32 值采用 big-endian 编码
