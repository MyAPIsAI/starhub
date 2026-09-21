package aws

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"testing"
)

// buildEventStreamFrame encodes one message the way Bedrock does (used to
// feed the reader in tests).
func buildEventStreamFrame(eventType string, payload []byte) []byte {
	headers := map[string]string{
		":message-type": "event",
		":event-type":   eventType,
		":content-type": "application/json",
	}
	var headerBytes []byte
	// deterministic order: content-type, event-type, message-type; the
	// value type is 7 (string) per the event stream spec
	for _, name := range []string{":content-type", ":event-type", ":message-type"} {
		value := headers[name]
		headerBytes = append(headerBytes, byte(len(name)))
		headerBytes = append(headerBytes, name...)
		headerBytes = append(headerBytes, 7)
		var lenBuf [2]byte
		binary.BigEndian.PutUint16(lenBuf[:], uint16(len(value)))
		headerBytes = append(headerBytes, lenBuf[:]...)
		headerBytes = append(headerBytes, value...)
	}

	totalLen := 12 + len(headerBytes) + len(payload) + 4
	msg := make([]byte, 0, totalLen)
	var prelude [12]byte
	binary.BigEndian.PutUint32(prelude[0:4], uint32(totalLen))
	binary.BigEndian.PutUint32(prelude[4:8], uint32(len(headerBytes)))
	binary.BigEndian.PutUint32(prelude[8:12], crc32.ChecksumIEEE(prelude[0:8]))
	msg = append(msg, prelude[:]...)
	msg = append(msg, headerBytes...)
	msg = append(msg, payload...)
	var crcBuf [4]byte
	binary.BigEndian.PutUint32(crcBuf[:], crc32.ChecksumIEEE(msg))
	msg = append(msg, crcBuf[:]...)
	return msg
}

func TestReadEventStreamMessage(t *testing.T) {
	stream := bytes.NewReader(append(
		buildEventStreamFrame("chunk", []byte(`{"type":"contentBlockDelta","delta":{"text":"hi"}}`)),
		buildEventStreamFrame("chunk", []byte(`{"type":"messageStop"}`))...,
	))

	msg, err := readEventStreamMessage(stream)
	if err != nil {
		t.Fatalf("read 1: %v", err)
	}
	if msg.EventType != "chunk" || msg.Headers[":content-type"] != "application/json" {
		t.Fatalf("msg 1 wrong: %+v", msg)
	}
	if string(msg.Payload) != `{"type":"contentBlockDelta","delta":{"text":"hi"}}` {
		t.Fatalf("payload 1 wrong: %s", msg.Payload)
	}

	msg, err = readEventStreamMessage(stream)
	if err != nil || msg.EventType != "chunk" {
		t.Fatalf("read 2: %+v %v", msg, err)
	}
}

func TestReadEventStreamRejectsCorruption(t *testing.T) {
	frame := buildEventStreamFrame("chunk", []byte("{}"))
	// corrupt the prelude CRC (byte 8..11)
	frame[8] ^= 0xFF
	if _, err := readEventStreamMessage(bytes.NewReader(frame)); err == nil {
		t.Fatal("corrupted prelude CRC must be rejected")
	}
	// truncated frame
	if _, err := readEventStreamMessage(bytes.NewReader(frame[:8])); err == nil {
		t.Fatal("truncated frame must error")
	}
}
