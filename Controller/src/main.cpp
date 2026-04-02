#include <Arduino.h>
#include <stdint.h>
#define RJVRX 34
#define RJVRY 35
#define LJVRX 32
#define LJVRY 33
#define RJSW 25
#define LJSW 26
#define TL 13
#define TR 27

int rjswIdle = HIGH;
int ljswIdle = HIGH;
uint8_t seq = 0;

struct ControllerPacket {
  uint8_t sof0;
  uint8_t sof1;
  uint8_t version;
  uint8_t sequence;
  uint16_t rjvrx_hundredths;
  uint16_t rjvry_hundredths;
  uint16_t ljvrx_hundredths;
  uint16_t ljvry_hundredths;
  uint8_t rjswPressed;
  uint8_t ljswPressed;
  uint8_t tlPressed;
  uint8_t trPressed;
  uint8_t checksum;
};

uint16_t adcToPercentHundredths(int raw) {
  // Map 0..4095 ADC to 0..10000 (% * 100), truncating beyond hundredths.
  return (uint16_t)((raw * 10000UL) / 4095UL);
}

uint8_t packetChecksum(const uint8_t* bytes, size_t lenWithoutChecksum) {
  uint8_t sum = 0;
  for (size_t i = 0; i < lenWithoutChecksum; i++) {
    sum ^= bytes[i];
  }
  return sum;
}

void setup() {
  pinMode(RJVRX, INPUT);
  pinMode(RJVRY, INPUT);
  pinMode(LJVRX, INPUT);
  pinMode(LJVRY, INPUT);
  pinMode(RJSW, INPUT_PULLUP);
  pinMode(LJSW, INPUT_PULLUP);
  pinMode(TL, INPUT);
  pinMode(TR, INPUT);
  Serial.begin(115200);

  delay(300);
  rjswIdle = digitalRead(RJSW);
  ljswIdle = digitalRead(LJSW);
}

void loop() {
  int rjvrxRaw = analogRead(RJVRX);
  int rjvryRaw = analogRead(RJVRY);
  int ljvrxRaw = analogRead(LJVRX);
  int ljvryRaw = analogRead(LJVRY);

  uint16_t rjvrx = adcToPercentHundredths(rjvrxRaw);
  uint16_t rjvry = adcToPercentHundredths(rjvryRaw);
  uint16_t ljvrx = adcToPercentHundredths(ljvrxRaw);
  uint16_t ljvry = adcToPercentHundredths(ljvryRaw);

  
  int rjswRaw = digitalRead(RJSW);
  int ljswRaw = digitalRead(LJSW);
  uint8_t rjswPressed = (rjswRaw != rjswIdle) ? 1 : 0;
  uint8_t ljswPressed = (ljswRaw != ljswIdle) ? 1 : 0;

  int tl = touchRead(TL);
  int tr = touchRead(TR);
  uint8_t tlPressed = (tl < 30) ? 1 : 0;
  uint8_t trPressed = (tr < 30) ? 1 : 0;

  ControllerPacket packet;
  packet.sof0 = 0xA5;
  packet.sof1 = 0x5A;
  packet.version = 1;
  packet.sequence = seq++;
  packet.rjvrx_hundredths = rjvrx;
  packet.rjvry_hundredths = rjvry;
  packet.ljvrx_hundredths = ljvrx;
  packet.ljvry_hundredths = ljvry;
  packet.rjswPressed = rjswPressed;
  packet.ljswPressed = ljswPressed;
  packet.trPressed = trPressed;
  packet.tlPressed = tlPressed;
   packet.checksum = packetChecksum((const uint8_t*)&packet, sizeof(packet) - 1);

  Serial.write((const uint8_t*)&packet, sizeof(packet));

  delay(5);
}