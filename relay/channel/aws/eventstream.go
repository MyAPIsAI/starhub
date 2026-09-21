package aws

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

// AWS event stream framing (the binary envelope of Bedrock
// invoke-with-response-stream responses). Layout per message:
//
//	+------------+---------------+-------------+---------+--------+
//	| total len  |  headers len  | prelude CRC | headers | payload | message CRC |
//	+------------+---------------+-------------+---------+--------+
//
// All integers are big-endian; both CRCs are CRC32-IEEE. Headers are typed
// TLV records; the ":event-type" header carries the event name.
type eventStreamMessage struct {
	EventType string // ":event-type" header value
	Headers   map[string]string
	Payload   []byte
}

const (
	maxEventStreamMessage = 16 << 20
	headerTypeString      = 7
)

var errMalformedEventStream = errors.New("malformed aws event stream frame")

// EncodeEventStreamMessage builds one framed message (chunk event with
// the given payload). Tests use it to fabricate Bedrock streams; it also
// documents the wire format next to the reader.
func EncodeEventStreamMessage(eventType string, payload []byte) []byte {
	var msg []byte
	appendHeader := func(name, value string) {
		msg = append(msg, byte(len(name)))
		msg = append(msg, name...)
		msg = append(msg, headerTypeString)
		var lenBuf [2]byte
		binary.BigEndian.PutUint16(lenBuf[:], uint16(len(value)))
		msg = append(msg, lenBuf[:]...)
		msg = append(msg, value...)
	}
	headerStart := len(msg)
	appendHeader(":message-type", "event")
	appendHeader(":event-type", eventType)
	appendHeader(":content-type", "application/json")
	headerBytes := msg[headerStart:]

	totalLen := 12 + len(headerBytes) + len(payload) + 4
	var prelude [12]byte
	binary.BigEndian.PutUint32(prelude[0:4], uint32(totalLen))
	binary.BigEndian.PutUint32(prelude[4:8], uint32(len(headerBytes)))
	binary.BigEndian.PutUint32(prelude[8:12], crc32.ChecksumIEEE(prelude[0:8]))
	out := make([]byte, 0, totalLen)
	out = append(out, prelude[:]...)
	out = append(out, headerBytes...)
	out = append(out, payload...)
	var crcBuf [4]byte
	binary.BigEndian.PutUint32(crcBuf[:], crc32.ChecksumIEEE(out))
	return append(out, crcBuf[:]...)
}

// readEventStreamMessage reads one framed message off the stream.
func readEventStreamMessage(r io.Reader) (*eventStreamMessage, error) {
	var prelude [12]byte
	if _, err := io.ReadFull(r, prelude[:]); err != nil {
		return nil, err
	}
	totalLen := binary.BigEndian.Uint32(prelude[0:4])
	headersLen := binary.BigEndian.Uint32(prelude[4:8])
	preludeCRC := binary.BigEndian.Uint32(prelude[8:12])
	if totalLen < 16 || uint64(totalLen) > maxEventStreamMessage || uint64(headersLen) > uint64(totalLen)-12 {
		return nil, fmt.Errorf("%w: bad lengths (total=%d headers=%d)", errMalformedEventStream, totalLen, headersLen)
	}
	if preludeCRC != crc32.ChecksumIEEE(prelude[0:8]) {
		return nil, fmt.Errorf("%w: prelude CRC mismatch", errMalformedEventStream)
	}
	headers := make([]byte, headersLen)
	if _, err := io.ReadFull(r, headers); err != nil {
		return nil, err
	}
	// totalLen covers prelude (12) + headers + payload + message CRC (4).
	payloadLen := int64(totalLen) - 12 - int64(headersLen) - 4
	if payloadLen < 0 {
		return nil, fmt.Errorf("%w: negative payload length", errMalformedEventStream)
	}
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	var msgCRC [4]byte
	if _, err := io.ReadFull(r, msgCRC[:]); err != nil {
		return nil, err
	}

	msg := &eventStreamMessage{Headers: map[string]string{}, Payload: payload}
	if err := parseEventStreamHeaders(headers, msg.Headers); err != nil {
		return nil, err
	}
	msg.EventType = msg.Headers[":event-type"]
	return msg, nil
}

// parseEventStreamHeaders decodes the TLV header block (value types per
// the AWS event stream spec: 0 true, 1 false, 2 byte, 3 short, 4 int,
// 5 long, 6 byte array, 7 string, 8 timestamp, 9 uuid).
func parseEventStreamHeaders(buf []byte, out map[string]string) error {
	for len(buf) > 0 {
		if len(buf) < 2 {
			return errMalformedEventStream
		}
		nameLen := int(buf[0])
		if len(buf) < 1+nameLen+1 {
			return errMalformedEventStream
		}
		name := string(buf[1 : 1+nameLen])
		buf = buf[1+nameLen:]
		valueType := buf[0]
		buf = buf[1:]
		switch valueType {
		case 0: // true
			out[name] = "true"
			continue
		case 1: // false
			out[name] = "false"
			continue
		case 2: // byte
			if len(buf) < 1 {
				return errMalformedEventStream
			}
			out[name] = fmt.Sprintf("%d", int8(buf[0]))
			buf = buf[1:]
			continue
		case 3: // short
			if len(buf) < 2 {
				return errMalformedEventStream
			}
			out[name] = fmt.Sprintf("%d", int16(binary.BigEndian.Uint16(buf[:2])))
			buf = buf[2:]
			continue
		case 4: // int
			if len(buf) < 4 {
				return errMalformedEventStream
			}
			out[name] = fmt.Sprintf("%d", int32(binary.BigEndian.Uint32(buf[:4])))
			buf = buf[4:]
			continue
		case 5: // long
			if len(buf) < 8 {
				return errMalformedEventStream
			}
			out[name] = fmt.Sprintf("%d", int64(binary.BigEndian.Uint64(buf[:8])))
			buf = buf[8:]
			continue
		case 6, 7: // byte array / string: 2-byte length prefix
			if len(buf) < 2 {
				return errMalformedEventStream
			}
			n := int(binary.BigEndian.Uint16(buf[:2]))
			if len(buf) < 2+n {
				return errMalformedEventStream
			}
			if valueType == 7 {
				out[name] = string(buf[2 : 2+n])
			}
			buf = buf[2+n:]
		case 8: // timestamp (8 bytes)
			if len(buf) < 8 {
				return errMalformedEventStream
			}
			buf = buf[8:]
		case 9: // uuid (16 bytes)
			if len(buf) < 16 {
				return errMalformedEventStream
			}
			buf = buf[16:]
		default:
			// an unknown type's length is not recoverable; refuse instead
			// of desyncing the stream.
			return fmt.Errorf("%w: unsupported header value type %d", errMalformedEventStream, valueType)
		}
	}
	return nil
}
