package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"go.bug.st/serial"
)

const (
	frameSize        = 17
	frameSOF0        = 0xA5
	frameSOF1        = 0x5A
	frameVersion     = 1
	axisMaxPercent   = 100
	axisMaxHundredth = 10000

	deadzoneMin = 30
	deadzoneMax = 60

	mouseAxisEpsilon = 0.03
	mouseFilterAlpha = 0.45

	inputMouse    = 0
	inputKeyboard = 1

	mouseeventfMove      = 0x0001
	mouseeventfLeftDown  = 0x0002
	mouseeventfLeftUp    = 0x0004
	mouseeventfRightDown = 0x0008
	mouseeventfRightUp   = 0x0010

	keyeventfKeyUp = 0x0002

	vkF13 = 0x7C
	vkF14 = 0x7D
	vkF15 = 0x7E
	vkF16 = 0x7F
	vkF17 = 0x80
	vkF18 = 0x81
)

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
)

type serialPacket struct {
	rightX     uint8
	rightY     uint8
	leftX      uint8
	leftY      uint8
	rightSW    uint8
	leftSW     uint8
	touchRight uint8
	touchLeft  uint8
}

type keyboardState struct {
	f13 bool
	f14 bool
	f15 bool
	f16 bool
	f17 bool
	f18 bool
}

type mouseState struct {
	leftDown  bool
	rightDown bool
}

type inputController struct {
	keys         keyboardState
	mouse        mouseState
	mouseMaxStep float64
	invertMouseY bool
	xCarry       float64
	yCarry       float64
	filterRx     float64
	filterRy     float64

	telemetryMu          sync.Mutex
	lastPacket           serialPacket
	lastSequence         uint8
	hasPacket            bool
	lastPacketAt         time.Time
	validPackets         uint64
	decodeDropped        uint64
	checksumMismatches   uint64
	checksumAcceptedBads uint64
}

type mouseInput struct {
	dx          int32
	dy          int32
	mouseData   uint32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// This layout matches INPUT on 64-bit Windows (4-byte type + 4-byte padding + 32-byte union).
type winInput struct {
	inputType uint32
	_         uint32
	data      [unsafe.Sizeof(mouseInput{})]byte
}

func main() {
	portName := flag.String("port", "COM4", "serial COM port")
	baud := flag.Int("baud", 115200, "serial baud rate")
	mouseMaxStep := flag.Float64("mouse-max-step", 8.0, "max mouse movement per packet at full stick")
	invertMouseY := flag.Bool("invert-mouse-y", true, "invert right joystick Y for mouse movement")
	debugPackets := flag.Bool("debug-packets", false, "print incoming packets")
	strictChecksum := flag.Bool("strict-checksum", false, "drop packets with checksum mismatch")
	flag.Parse()

	mode := &serial.Mode{
		BaudRate: *baud,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(*portName, mode)
	if err != nil {
		log.Fatalf("failed to open %s: %v", *portName, err)
	}
	defer port.Close()

	controller := &inputController{
		mouseMaxStep: *mouseMaxStep,
		invertMouseY: *invertMouseY,
	}
	defer controller.releaseAll()

	done := make(chan struct{})
	defer close(done)
	go startStatusLogger(controller, done)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	errCh := make(chan error, 1)
	go func() {
		errCh <- readSerialLoop(port, controller, *debugPackets, *strictChecksum)
	}()

	log.Printf("listening on %s @ %d baud", *portName, *baud)

	select {
	case <-interrupt:
		log.Println("interrupt received, exiting")
	case err := <-errCh:
		if err != nil {
			log.Printf("serial loop ended: %v", err)
		}
	}
}

func readSerialLoop(port serial.Port, controller *inputController, debugPackets bool, strictChecksum bool) error {
	one := make([]byte, 1)
	frame := make([]byte, 0, frameSize)

	for {
		n, err := io.ReadFull(port, one)
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}

		frame = append(frame, one[0])
		if len(frame) < frameSize {
			continue
		}
		if len(frame) > frameSize {
			frame = frame[1:]
		}

		packet, ok, checksumOK := decodeFrame(frame)
		if !ok {
			controller.recordDecodeDropped()
			continue
		}
		if !checksumOK {
			controller.recordChecksumMismatch(!strictChecksum)
			if strictChecksum {
				if debugPackets {
					log.Printf("checksum mismatch on seq=%d", frame[3])
				}
				continue
			}
		}

		controller.recordValidPacket(frame[3], packet)

		if debugPackets {
			log.Printf("SEQ=%d RX=%d RY=%d LX=%d LY=%d RSW=%d LSW=%d TR=%d TL=%d", frame[3], packet.rightX, packet.rightY, packet.leftX, packet.leftY, packet.rightSW, packet.leftSW, packet.touchRight, packet.touchLeft)
		}

		controller.apply(packet)
		frame = frame[:0]
	}
}

type telemetrySnapshot struct {
	hasPacket            bool
	lastPacket           serialPacket
	lastSequence         uint8
	ageMs                int64
	validPackets         uint64
	decodeDropped        uint64
	checksumMismatches   uint64
	checksumAcceptedBads uint64
}

func startStatusLogger(controller *inputController, done <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			s := controller.snapshot()
			if !s.hasPacket {
				log.Printf("STATUS waiting for valid packet | valid=%d decodeDropped=%d checksumBad=%d checksumAccepted=%d", s.validPackets, s.decodeDropped, s.checksumMismatches, s.checksumAcceptedBads)
				continue
			}

			log.Printf("STATUS seq=%d RX=%d RY=%d LX=%d LY=%d RSW=%d LSW=%d TR=%d TL=%d age=%dms | valid=%d decodeDropped=%d checksumBad=%d checksumAccepted=%d", s.lastSequence, s.lastPacket.rightX, s.lastPacket.rightY, s.lastPacket.leftX, s.lastPacket.leftY, s.lastPacket.rightSW, s.lastPacket.leftSW, s.lastPacket.touchRight, s.lastPacket.touchLeft, s.ageMs, s.validPackets, s.decodeDropped, s.checksumMismatches, s.checksumAcceptedBads)
		}
	}
}

func (c *inputController) recordDecodeDropped() {
	c.telemetryMu.Lock()
	c.decodeDropped++
	c.telemetryMu.Unlock()
}

func (c *inputController) recordChecksumMismatch(accepted bool) {
	c.telemetryMu.Lock()
	c.checksumMismatches++
	if accepted {
		c.checksumAcceptedBads++
	}
	c.telemetryMu.Unlock()
}

func (c *inputController) recordValidPacket(sequence uint8, packet serialPacket) {
	c.telemetryMu.Lock()
	c.validPackets++
	c.lastSequence = sequence
	c.lastPacket = packet
	c.lastPacketAt = time.Now()
	c.hasPacket = true
	c.telemetryMu.Unlock()
}

func (c *inputController) snapshot() telemetrySnapshot {
	c.telemetryMu.Lock()
	defer c.telemetryMu.Unlock()

	s := telemetrySnapshot{
		hasPacket:            c.hasPacket,
		lastPacket:           c.lastPacket,
		lastSequence:         c.lastSequence,
		validPackets:         c.validPackets,
		decodeDropped:        c.decodeDropped,
		checksumMismatches:   c.checksumMismatches,
		checksumAcceptedBads: c.checksumAcceptedBads,
	}

	if c.hasPacket {
		s.ageMs = time.Since(c.lastPacketAt).Milliseconds()
	}

	return s
}

func decodeFrame(frame []byte) (serialPacket, bool, bool) {
	var packet serialPacket

	if len(frame) != frameSize {
		return packet, false, false
	}

	if frame[0] != frameSOF0 || frame[1] != frameSOF1 || frame[2] != frameVersion {
		return packet, false, false
	}

	rightXHundredths := binary.LittleEndian.Uint16(frame[4:6])
	rightYHundredths := binary.LittleEndian.Uint16(frame[6:8])
	leftXHundredths := binary.LittleEndian.Uint16(frame[8:10])
	leftYHundredths := binary.LittleEndian.Uint16(frame[10:12])
	if rightXHundredths > axisMaxHundredth || rightYHundredths > axisMaxHundredth || leftXHundredths > axisMaxHundredth || leftYHundredths > axisMaxHundredth {
		return packet, false, false
	}

	if frame[12] > 1 || frame[13] > 1 || frame[14] > 1 || frame[15] > 1 {
		return packet, false, false
	}

	packet.rightX = hundredthsToPercent(rightXHundredths)
	packet.rightY = hundredthsToPercent(rightYHundredths)
	packet.leftX = hundredthsToPercent(leftXHundredths)
	packet.leftY = hundredthsToPercent(leftYHundredths)
	packet.rightSW = frame[12]
	packet.leftSW = frame[13]
	packet.touchRight = frame[14]
	packet.touchLeft = frame[15]

	checksumOK := xorChecksum(frame[:16]) == frame[16]

	return packet, true, checksumOK
}

func hundredthsToPercent(v uint16) uint8 {
	if v > axisMaxHundredth {
		v = axisMaxHundredth
	}

	return uint8((uint32(v) + 50) / 100)
}

func xorChecksum(data []byte) uint8 {
	var sum uint8
	for i := 0; i < len(data); i++ {
		sum ^= data[i]
	}

	return sum
}

func (c *inputController) apply(packet serialPacket) {
	rx := applyCurve(normalizeAxis(packet.rightX))
	ry := applyCurve(normalizeAxis(packet.rightY))
	rx = zeroIfTiny(rx)
	ry = zeroIfTiny(ry)

	// soft filtering to smooth stick movement and reduce frame jitter
	c.filterRx = c.filterRx*(1-mouseFilterAlpha) + rx*mouseFilterAlpha
	c.filterRy = c.filterRy*(1-mouseFilterAlpha) + ry*mouseFilterAlpha

	c.updateFunctionKeys(packet.leftX, packet.leftY, packet.touchRight, packet.touchLeft)
	c.updateMouseButtons(packet.leftSW != 0, packet.rightSW != 0)
	c.updateMouseMove(c.filterRx, c.filterRy)
}

func zeroIfTiny(v float64) float64 {
	if math.Abs(v) < mouseAxisEpsilon {
		return 0
	}

	return v
}

func normalizeAxis(v uint8) float64 {
	if v >= deadzoneMin && v <= deadzoneMax {
		return 0
	}

	if v < deadzoneMin {
		return -float64(int(deadzoneMin)-int(v)) / float64(deadzoneMin)
	}

	return float64(int(v)-int(deadzoneMax)) / float64(axisMaxPercent-deadzoneMax)
}

func applyCurve(v float64) float64 {
	if v == 0 {
		return 0
	}

	abs := math.Abs(v)
	curved := math.Pow(abs, 1.7)
	if v < 0 {
		return -curved
	}

	return curved
}

func (c *inputController) updateFunctionKeys(leftX, leftY, touchRight, touchLeft uint8) {
	wantedF13 := leftX > deadzoneMax
	wantedF14 := leftX < deadzoneMin
	wantedF15 := leftY > deadzoneMax
	wantedF16 := leftY < deadzoneMin
	wantedF17 := touchRight != 0
	wantedF18 := touchLeft != 0

	c.setKeyState(vkF13, &c.keys.f13, wantedF13)
	c.setKeyState(vkF14, &c.keys.f14, wantedF14)
	c.setKeyState(vkF15, &c.keys.f15, wantedF15)
	c.setKeyState(vkF16, &c.keys.f16, wantedF16)
	c.setKeyState(vkF17, &c.keys.f17, wantedF17)
	c.setKeyState(vkF18, &c.keys.f18, wantedF18)
}

func (c *inputController) updateMouseButtons(leftPressed, rightPressed bool) {
	c.setMouseButtonState(&c.mouse.leftDown, leftPressed, mouseeventfLeftDown, mouseeventfLeftUp)
	c.setMouseButtonState(&c.mouse.rightDown, rightPressed, mouseeventfRightDown, mouseeventfRightUp)
}

func (c *inputController) updateMouseMove(ry, rx float64) {
	if c.invertMouseY {
		ry = -ry
	}

	c.xCarry += rx * c.mouseMaxStep
	c.yCarry += ry * c.mouseMaxStep

	dx := int32(c.xCarry)
	dy := int32(c.yCarry)

	c.xCarry -= float64(dx)
	c.yCarry -= float64(dy)

	if dx == 0 && dy == 0 {
		return
	}

	if err := sendMouseMove(dx, dy); err != nil {
		log.Printf("mouse move failed: %v", err)
	}
}

func (c *inputController) setKeyState(vk uint16, current *bool, wanted bool) {
	if *current == wanted {
		return
	}

	if err := sendKeyboard(vk, !wanted); err != nil {
		log.Printf("keyboard event failed for VK 0x%X: %v", vk, err)
		return
	}

	*current = wanted
}

func (c *inputController) setMouseButtonState(current *bool, wanted bool, downFlag uint32, upFlag uint32) {
	if *current == wanted {
		return
	}

	flag := upFlag
	if wanted {
		flag = downFlag
	}

	if err := sendMouseButton(flag); err != nil {
		log.Printf("mouse button event failed (flag 0x%X): %v", flag, err)
		return
	}

	*current = wanted
}

func (c *inputController) releaseAll() {
	c.setKeyState(vkF13, &c.keys.f13, false)
	c.setKeyState(vkF14, &c.keys.f14, false)
	c.setKeyState(vkF15, &c.keys.f15, false)
	c.setKeyState(vkF16, &c.keys.f16, false)
	c.setKeyState(vkF17, &c.keys.f17, false)
	c.setKeyState(vkF18, &c.keys.f18, false)

	c.setMouseButtonState(&c.mouse.leftDown, false, mouseeventfLeftDown, mouseeventfLeftUp)
	c.setMouseButtonState(&c.mouse.rightDown, false, mouseeventfRightDown, mouseeventfRightUp)
}

func sendMouseMove(dx, dy int32) error {
	var in winInput
	in.inputType = inputMouse

	mi := (*mouseInput)(unsafe.Pointer(&in.data[0]))
	mi.dx = dx
	mi.dy = dy
	mi.dwFlags = mouseeventfMove

	return sendInput(&in)
}

func sendMouseButton(flag uint32) error {
	var in winInput
	in.inputType = inputMouse

	mi := (*mouseInput)(unsafe.Pointer(&in.data[0]))
	mi.dwFlags = flag

	return sendInput(&in)
}

func sendKeyboard(vk uint16, keyUp bool) error {
	var in winInput
	in.inputType = inputKeyboard

	ki := (*keybdInput)(unsafe.Pointer(&in.data[0]))
	ki.wVk = vk
	if keyUp {
		ki.dwFlags = keyeventfKeyUp
	}

	return sendInput(&in)
}

func sendInput(in *winInput) error {
	r1, _, callErr := procSendInput.Call(
		1,
		uintptr(unsafe.Pointer(in)),
		uintptr(unsafe.Sizeof(*in)),
	)
	if r1 == 0 {
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return fmt.Errorf("SendInput returned 0")
	}

	return nil
}
