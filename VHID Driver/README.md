## Controller VHID Driver

This is a a Virtual Human Interface Device, it uses go-hid and go-serial to read the serial data from the ESP32 controller and emulates a virtual HID device on the PC. The driver reads the serial data in the format sent by the ESP32 controller and updates the state of the virtual HID device accordingly.

## Key Mapping

- Left stick X+ -> F13
- Left stick X- -> F14
- Left stick Y+ -> F15
- Left stick Y- -> F16
- Touch right -> F17
- Touch left -> F18

## Serial Frame

Frame has 13 fields (17 bytes total):

1. SOF0 (0xA5)
2. SOF1 (0x5A)
3. Version
4. Sequence
5. Right X (uint16 LE)
6. Right Y (uint16 LE)
7. Left X (uint16 LE)
8. Left Y (uint16 LE)
9. Right switch (0/1)
10. Left switch (0/1)
11. Touch right (0/1)
12. Touch left (0/1)
13. Checksum (XOR of bytes 0..15)