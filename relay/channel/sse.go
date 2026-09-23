package channel

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

// SSELine is one parsed server-sent-event frame (an "event:" name plus a
// "data:" payload; either may be empty).
type SSELine struct {
	Event string
	Data  string
}

// SSEReader incrementally reads an SSE body. It tolerates the framing
// quirks of the wild (CRLF, blank lines, multi-line data).
type SSEReader struct {
	r     *bufio.Reader
	event string
}

// NewSSEReader wraps an SSE body.
func NewSSEReader(r io.Reader) *SSEReader {
	return &SSEReader{r: bufio.NewReaderSize(r, 64*1024)}
}

// Next returns the next complete event. It returns io.EOF when the stream
// ended.
func (s *SSEReader) Next() (SSELine, error) {
	data := strings.Builder{}
	haveData := false
	for {
		line, err := s.r.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			switch {
			case trimmed == "":
				// blank line terminates the current event
				if haveData {
					out := SSELine{Event: s.event, Data: data.String()}
					s.event = ""
					return out, nil
				}
			case strings.HasPrefix(trimmed, ":"):
				// comment / keepalive
			case strings.HasPrefix(trimmed, "event:"):
				s.event = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			case strings.HasPrefix(trimmed, "data:"):
				if haveData {
					data.WriteByte('\n')
				}
				data.WriteString(strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
				haveData = true
			}
		}
		if err != nil {
			if err == io.EOF && haveData {
				out := SSELine{Event: s.event, Data: data.String()}
				s.event = ""
				return out, nil
			}
			return SSELine{}, err
		}
	}
}

var (
	ssePrefixEvent = []byte("event:")
	ssePrefixData  = []byte("data:")
	ssePrefixColon = []byte(":")
)

// SSEByte is one parsed server-sent-event frame with byte payloads.
type SSEByte struct {
	Event []byte
	Data  []byte
}

// SSEByteReader is a buffer-reusing sibling of SSEReader for byte
// consumers (the image relay, P7.1): Event and Data alias reader-owned
// scratch buffers, so one buffer serves every frame of a stream instead
// of allocating a fresh megabyte string per event.
//
// Single-owner contract: the returned slices stay valid only until the
// next Next call. Forward or parse the frame before reading the next one.
type SSEByteReader struct {
	r     *bufio.Reader
	event []byte
	data  []byte
	line  []byte
}

// NewSSEByteReader wraps an SSE body.
func NewSSEByteReader(r io.Reader) *SSEByteReader {
	return &SSEByteReader{
		r:     bufio.NewReaderSize(r, 64*1024),
		event: make([]byte, 0, 64),
		data:  make([]byte, 0, 64*1024),
	}
}

// Next returns the next complete event. It returns io.EOF when the
// stream ended. Framing quirks are tolerated like SSEReader: CRLF, blank
// lines, multi-line data, comment/keepalive lines. data: lines are
// detected by peek and accumulate their payload directly into the reused
// data buffer — the megabyte hot case never round-trips through the
// line buffer.
func (s *SSEByteReader) Next() (SSEByte, error) {
	s.data = s.data[:0]
	haveData := false
	for {
		if b, _ := s.r.Peek(len(ssePrefixData)); len(b) >= len(ssePrefixData) && bytes.Equal(b, ssePrefixData) {
			if haveData {
				s.data = append(s.data, '\n')
			}
			_, _ = s.r.Discard(len(ssePrefixData))
			if err := s.appendDataPayload(); err != nil {
				return SSEByte{}, err
			}
			haveData = true
			continue
		}
		line, lineErr := s.readLine()
		if len(line) > 0 {
			trimmed := bytes.TrimRight(line, "\r\n")
			switch {
			case len(trimmed) == 0:
				// blank line terminates the current event
				if haveData {
					out := SSEByte{Event: s.event, Data: s.data}
					s.event = s.event[:0]
					return out, nil
				}
			case bytes.HasPrefix(trimmed, ssePrefixColon):
				// comment / keepalive
			case bytes.HasPrefix(trimmed, ssePrefixEvent):
				s.event = append(s.event[:0], bytes.TrimSpace(trimmed[len(ssePrefixEvent):])...)
			case bytes.HasPrefix(trimmed, ssePrefixData):
				// Peek could not classify it (end of stream mid-prefix):
				// take the slow path through the line buffer.
				if haveData {
					s.data = append(s.data, '\n')
				}
				s.data = append(s.data, bytes.TrimSpace(trimmed[len(ssePrefixData):])...)
				haveData = true
			}
		}
		if lineErr != nil {
			if lineErr == io.EOF && haveData {
				out := SSEByte{Event: s.event, Data: s.data}
				s.event = s.event[:0]
				return out, nil
			}
			return SSEByte{}, lineErr
		}
	}
}

// appendDataPayload reads the remainder of a "data:"-prefixed line (the
// prefix itself is already discarded) and appends its trimmed payload
// to the reused data buffer. EOF is not an error here: it surfaces on
// the caller's next read.
func (s *SSEByteReader) appendDataPayload() error {
	start := len(s.data)
	for {
		chunk, err := s.r.ReadSlice('\n')
		// Geometric growth keeps cumulative allocations bounded for large
		// image frames. The default append growth becomes much smaller at
		// megabyte sizes, repeatedly copying the entire payload.
		if len(chunk) > cap(s.data)-len(s.data) {
			grown := make([]byte, len(s.data), max(2*cap(s.data), len(s.data)+len(chunk)))
			copy(grown, s.data)
			s.data = grown
		}
		s.data = append(s.data, chunk...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil && err != io.EOF {
			return err
		}
		break
	}
	// Trim this line's edges (newline bytes + surrounding whitespace),
	// matching strings.TrimSpace on the payload.
	seg := bytes.TrimSpace(s.data[start:])
	s.data = append(s.data[:start], seg...)
	return nil
}

// readLine reads one \n-terminated line into the reused line buffer. A
// data line can exceed the bufio buffer, so ErrBufferFull chunks are
// accumulated into the same reused buffer.
func (s *SSEByteReader) readLine() ([]byte, error) {
	s.line = s.line[:0]
	for {
		chunk, err := s.r.ReadSlice('\n')
		if len(chunk) > 0 {
			s.line = append(s.line, chunk...)
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return s.line, err
	}
}
