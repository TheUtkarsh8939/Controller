# DIY ESP32 Wireless Game Controller

A complete DIY game controller system using an ESP32 microcontroller with dual joysticks and capacitive touch buttons. The controller communicates via serial to a Windows driver that emulates keyboard and mouse input.

## Project Overview

This project consists of two main components:

1. **Controller Firmware** (`Controller/`) - ESP32 firmware that reads analog joysticks and capacitive touch inputs
2. **VHID Driver** (`VHID Driver/`) - Windows application that reads serial data and emulates mouse/keyboard input

### Features

- **Dual Analog Joysticks**: Independent X/Y axes on left and right controllers
- **Joystick Buttons**: Clickable switches on each joystick
- **Capacitive Touch Buttons**: Two touch-sensitive buttons for additional input
- **Serial Communication**: Binary protocol with checksums for reliable data transfer
- **Windows Integration**: Emulates mouse movement and keyboard input via Windows SendInput API
- **Telemetry**: Real-time packet statistics and diagnostics

---

## Hardware

### Bill of Materials

- **ESP32 Development Board** (ESP32-DOIT-DevKit-V1 or compatible)
- **2x Analog Joysticks** (3-axis: VRx, VRy, SW)
- **2x Capacitive Touch Buttons** (or any touch-sensitive GPIO)
- **USB Serial Adapter** (optional, if not using built-in USB on ESP32)
- **Connecting Wires** and breadboard/circuit board

### Pinout Diagram

```
ESP32 Pinout Mapping:

┌─────────────────────────────────────┐
│           ESP32 Board               │
├─────────────────────────────────────┤
│                                     │
│  Analog Inputs (ADC):               │
│  ┌─────────────────────────────┐    │
│  │ GPIO 32 ──── Left  Joystick X(Vrx)   │
│  │ GPIO 33 ──── Left  Joystick Y(Vry)   │
│  │ GPIO 34 ──── Right Joystick X(Vrx)   │
│  │ GPIO 35 ──── Right Joystick Y(Vry)   │
│  └─────────────────────────────┘    │
│                                     │
│  Digital Inputs (Buttons):          │
│  ┌─────────────────────────────┐    │
│  │ GPIO 26 ──── Left  Joystick SW, Connected to Joystick's onboard switch   │
│  │ GPIO 25 ──── Right Joystick SW, Connected to Joystick's onboard switch   │
│  └─────────────────────────────┘    │
│                                     │
│  Touch Inputs:                      │
│  ┌─────────────────────────────┐    │
│  │ GPIO 13 ──── Touch Left (TL, A floating Jumper wire)     │
│  │ GPIO 27 ──── Touch Right (TR, A floating Jumper wire)    │
│  └─────────────────────────────┘    │
│                                     │
│  Serial Communication:              │
│  ┌─────────────────────────────┐    │
│  │ TX ──── to Windows COM port      │
│  │ RX ──── (not used)               │
│  └─────────────────────────────┘    │
│                                     │
└─────────────────────────────────────┘
```
<img width="1919" height="1079" alt="Image" src="https://github.com/user-attachments/assets/1241e1a7-b9bd-4c79-a983-d9469704f3ab" />

### Pin Details

| Component | GPIO | ADC | Function | Notes |
|-----------|------|-----|----------|-------|
| Left Joystick X | 32 | ADC1_4 | Analog Input | 0-4095 mapped to 0-10000 |
| Left Joystick Y | 33 | ADC1_5 | Analog Input | 0-4095 mapped to 0-10000 |
| Right Joystick X | 34 | ADC1_6 | Analog Input | 0-4095 mapped to 0-10000 |
| Right Joystick Y | 35 | ADC1_7 | Analog Input | 0-4095 mapped to 0-10000 |
| Left Joystick Button | 26 | — | Digital Input | INPUT_PULLUP, active LOW |
| Right Joystick Button | 25 | — | Digital Input | INPUT_PULLUP, active LOW |
| Touch Left | 13 | — | Touch Input | Capacitive, threshold < 30 |
| Touch Right | 27 | — | Touch Input | Capacitive, threshold < 30 |
| Serial TX | — | — | UART0 Default | 115200 baud |

---

## Software Architecture

### 1. ESP32 Controller Firmware

**Location**: `Controller/src/main.cpp`

**Functionality**:
- Reads analog joystick values (0-4095) and maps to percentage hundredths (0-10000)
- Reads digital joystick switch states with debouncing via idle state detection
- Reads capacitive touch button states (threshold < 30)
- Packs data into a binary protocol with checksum
- Transmits 17-byte packets at 200Hz (5ms interval) via Serial

**Protocol Format**:
```
Offset | Size | Field               | Example Values
-------|------|---------------------|----------------
0      | 1    | SOF0 (0xA5)         | 0xA5
1      | 1    | SOF1 (0x5A)         | 0x5A
2      | 1    | Version             | 0x01
3      | 1    | Sequence Number     | 0x00-0xFF (auto-increment)
4-5    | 2    | Right Joystick X    | 0x0000-0x2710 (uint16_t LE)
6-7    | 2    | Right Joystick Y    | 0x0000-0x2710 (uint16_t LE)
8-9    | 2    | Left Joystick X     | 0x0000-0x2710 (uint16_t LE)
10-11  | 2    | Left Joystick Y     | 0x0000-0x2710 (uint16_t LE)
12     | 1    | Right Button        | 0x00 or 0x01
13     | 1    | Left Button         | 0x00 or 0x01
14     | 1    | Touch Right         | 0x00 or 0x01
15     | 1    | Touch Left          | 0x00 or 0x01
16     | 1    | Checksum (XOR)      | XOR of bytes 0-15
```

**Build & Upload**:
```bash
cd Controller
platformio run --target upload
platformio device monitor --baud 115200
```

### 2. Windows VHID Driver

**Location**: `VHID Driver/main.go`

**Functionality**:
- Reads serial data from COM port (115200 baud, 8N1)
- Decodes binary packets and validates checksums
- Maps right joystick to mouse movement with:
  - Deadzone filtering (30-60 hundredths)
  - Exponential smoothing (alpha = 0.45)
  - Configurable max movement per packet (default 8 pixels)
  - Optional Y-axis inversion
- Maps buttons/touch to keyboard keys:
  - Right Joystick Button → F13 key
  - Left Joystick Button → F14 key
  - Touch Right → F15 key
  - Touch Left → F16 key
- Includes telemetry logging for packet statistics

**Command Line Options**:
```
-port string              Serial COM port (default "COM4")
-baud int                 Baud rate (default 115200)
-mouse-max-step float     Max mouse movement per packet (default 8.0)
-invert-mouse-y bool      Invert Y-axis for mouse (default true)
-debug-packets bool       Print incoming packets
-strict-checksum bool     Drop packets with bad checksum (default false)
```

**Usage Examples**:
```bash
# Basic usage with defaults (COM4 @ 115200 baud)
./VHID_Driver.exe

# Use COM5, strict checksum validation
./VHID_Driver.exe -port COM5 -strict-checksum=true

# Debug mode with packet printing
./VHID_Driver.exe -debug-packets=true

# Adjust mouse sensitivity (16 pixels max per packet)
./VHID_Driver.exe -mouse-max-step=16.0

# Don't invert mouse Y axis
./VHID_Driver.exe -invert-mouse-y=false
```

**Build**:
```bash
cd "VHID Driver"
go build -o VHID_Driver.exe
```

---

## Communication Protocol

### Packet Structure

Each packet is 17 bytes transmitted over UART at 115200 baud, 8 data bits, 1 stop bit, no parity.

- **Start of Frame (SOF)**: 0xA5 0x5A (2 bytes) - Frame synchronization markers
- **Version**: 0x01 (1 byte) - Protocol version
- **Sequence**: Auto-incrementing counter (1 byte) - Detects dropped packets
- **Joystick Data**: 4 × uint16_t (8 bytes) - X/Y values (0-10000)
- **Button States**: 4 bytes - Right/Left/Touch Right/Touch Left (0 or 1)
- **Checksum**: XOR of all previous bytes (1 byte) - Error detection

### Transmission Rate

- **Frequency**: 200 Hz (5ms interval)
- **Throughput**: ~29.4 kbps (17 bytes × 200 packets/sec × 8 bits)

---

## Setup Instructions

### Hardware Assembly

1. **Connect Joysticks**:
   - Left Joystick VRx → GPIO 32
   - Left Joystick VRy → GPIO 33
   - Left Joystick SW → GPIO 26 (with pull-up resistor if needed)
   - Right Joystick VRx → GPIO 34
   - Right Joystick VRy → GPIO 35
   - Right Joystick SW → GPIO 25 (with pull-up resistor if needed)

2. **Connect Touch Buttons**:
   - Touch Left → GPIO 13
   - Touch Right → GPIO 27

3. **Power & Serial**:
   - Connect ESP32 to computer via USB (or external serial adapter)
   - Note the COM port number

### ESP32 Firmware

1. Install [PlatformIO](https://platformio.org/) in VS Code
2. Clone/open the project
3. Build and upload:
   ```bash
   cd Controller
   platformio run --target upload
   ```
4. Monitor serial output:
   ```bash
   platformio device monitor --baud 115200
   ```

### Windows Driver

1. Identify the COM port (COM Device Manager or from PlatformIO monitor)
2. Build the Go driver:
   ```bash
   cd "VHID Driver"
   go build -o VHID_Driver.exe
   ```
3. Run the driver:
   ```bash
   ./VHID_Driver.exe -port COM4
   ```
4. Start moving the joysticks to control mouse movement

---

## Troubleshooting

### Controller Not Detected

- **Check serial connection**: Ensure USB cable is properly connected
- **Check COM port**: Use Device Manager to find the COM port number
- **Update drivers**: ESP32 may require CH340 or CP2102 drivers

### Packet Corruption

- **Checksum mismatches**: Loose wire connection or USB power issue
- **Solution**: Add `-debug-packets=true` to see real-time packet data
- **Strict mode**: Use `-strict-checksum=true` to drop bad packets instead of accepting them

### Mouse Movement Erratic

- **Increase deadzone threshold**: Joystick may have drift
- **Adjust filter alpha**: Increase `mouseFilterAlpha` in VHID driver for more smoothing
- **Check stick calibration**: Ensure joysticks are centered at rest

### No Mouse Movement

- **Verify baud rate**: Must match (default 115200 on both ends)
- **Check button mapping**: F13-F16 keys may not be recognized in some applications
- **Run as Administrator**: Windows may require elevated privileges for SendInput

---

## Customization

### Adjust Mouse Sensitivity

Edit in `VHID Driver/main.go`:
```go
mouseMaxStep := flag.Float64("mouse-max-step", 8.0, "...")
```
- Lower values = slower mouse (e.g., 4.0 for fine control)
- Higher values = faster mouse (e.g., 16.0 for quick movements)

### Change Key Mappings

In `VHID Driver/main.go`, modify the `apply()` function:
```go
vkF13 = 0x7C  // Right Joystick Button
vkF14 = 0x7D  // Left Joystick Button
vkF15 = 0x7E  // Touch Right
vkF16 = 0x7F  // Touch Left
```

### Adjust Touch Sensitivity

In `Controller/src/main.cpp`:
```cpp
uint8_t tlPressed = (tl < 30) ? 1 : 0;  // Lower = more sensitive
uint8_t trPressed = (tr < 30) ? 1 : 0;
```

---

## Telemetry & Diagnostics

The Windows driver outputs status every second:

```
STATUS seq=42 RX=5000 RY=5000 LX=2500 LY=7500 RSW=0 LSW=1 TR=0 TL=0 age=125ms | valid=1230 decodeDropped=0 checksumBad=0 checksumAccepted=0
```

- **seq**: Packet sequence number (monotonic)
- **RX/RY/LX/LY**: Joystick positions (0-10000)
- **RSW/LSW**: Joystick button states
- **TR/TL**: Touch button states
- **age**: Time since last valid packet (ms)
- **valid**: Total valid packets received
- **decodeDropped**: Misaligned frames discarded
- **checksumBad**: Packets with invalid checksums
- **checksumAccepted**: Bad checksums accepted (when not in strict mode)

---

## Performance Specifications

| Metric | Value |
|--------|-------|
| Packet Rate | 200 Hz |
| Update Latency | ~5-10 ms |
| Baud Rate | 115200 bps |
| ADC Resolution | 12-bit (0-4095) |
| Joystick Resolution | Millipercent (0-10000) |
| Button Latency | <10 ms |
| Touch Detection Threshold | <30  |

---

## License

This project is designed as a DIY hobby controller. Feel free to modify and customize for your needs.

---

## Support & Debugging

### Enable Debug Output

Run driver with debug flags:
```bash
./VHID_Driver.exe -debug-packets=true -strict-checksum=true
```

Expected output:
```
SEQ=0 RX=5000 RY=5000 LX=2500 LY=7500 RSW=0 LSW=0 TR=0 TL=0
SEQ=1 RX=5001 RY=4999 LX=2501 LY=7499 RSW=0 LSW=0 TR=0 TL=0
```

### Common Issues Log

| Issue | Cause | Solution |
|-------|-------|----------|
| No packets received | Wrong COM port | Check Device Manager |
| Frequent checksum errors | Loose connections | Secure all wiring |
| Mouse stutters | USB power issue | Use powered USB hub |
| Buttons not working | Key mapping issue | Verify F13-F16 in application |
| Erratic movement | Joystick drift | Calibrate sticks or increase deadzone |

---



