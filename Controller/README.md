# DIY CONTROLER
Controllers have been pretty expesnive these days. So I decided to make one myself, this code here is the MCU code for ESP32 controller.
This just reads the inputs and sends it serially as esp32 does not have native HID support. The code is pretty simple and self explanatory, it reads the inputs and sends it serially in a specific format: 
```
RightJoystickX,RightJoystickY,LeftJoystickX,LeftJoystickY,RightJoystickButton,LeftJoystickButton,TouchRight,TouchLeft
```
Where the joystick values are between 0 and 100 and the button values are either 0 or 1.

The rest is handled by the vhid driver on the PC side, which reads the serial data and emulates a virtual HID device based on the received data.