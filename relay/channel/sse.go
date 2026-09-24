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

const maxPooledSSEFrame = 4 << 20

// Limit retained frame storage while sharing large buffers across workers.
var sseFramePool = make(chan []byte, 4)

// NewSSEByteReader wraps an SSE body.
func NewSSEByteReader(r io.Reader) *SSEByteReader {
	var buf []byte
	select {
	case buf = <-sseFramePool:
	default:
		buf = make([]byte, 0, 64*1024)
	}
	return &SSEByteReader{
		r:     bufio.NewReaderSize(r, 64*1024),
		event: make([]byte, 0, 64),
		data:  buf[:0],
	}
}

// Release makes the frame buffer available to another stream. The reader and
// previously returned Event and Data slices must not be used after this call.
func (s *SSEByteReader) Release() {
	if s.data == nil {
		return
	}
	if cap(s.data) <= maxPooledSSEFrame {
		buf := s.data[:0]
		select {
		case sseFramePool <- buf:
		default:
		}
	}
	s.data = nil
}

// Next returns the next complete event. It returns io.EOF when the
// stream ended. Framing quirks are tolerated like SSEReader: CRLF, blank
// lines, multi-line data, comment/keepalive lines. data: lines are
// detected by peek and accumulate their payload directly into the reused
// frame buffer. Oversized frames collect overflow chunks and consolidate
// once, rather than repeatedly reallocating and copying the whole frame.
func (s *SSEByteReader) Next() (SSEByte, error) {
	frame := sseFrameBuffer{tail: s.data[:0]}
	var lineSpans [8][2]int
	spans := lineSpans[:0]
	haveData := false
	finish := func() SSEByte {
		s.data = frame.bytes()
		out := s.data[:0]
		// Trim after consolidation so whitespace and UTF-8 sequences that
		// cross chunk boundaries retain the original framing semantics.
		for i, span := range spans {
			part := bytes.TrimSpace(s.data[span[0]:span[1]])
			if i > 0 {
				out = append(out, '\n')
			}
			out = append(out, part...)
		}
		s.data = out
		result := SSEByte{Event: s.event, Data: s.data}
		s.event = s.event[:0]
		return result
	}
	for {
		// Peek a single byte before the multi-byte "data:" prefix check:
		// Peek(5) blocks until 5 bytes are buffered, stalling event
		// delivery when only the 1-byte blank-line terminator is available
		// (the next frame has not arrived yet). By checking the first byte
		// we skip straight to readLine for blank lines, event:, comments,
		// and any non-"data:" line without waiting for more input.
		first, _ := s.r.Peek(1)
		if len(first) > 0 && first[0] == 'd' {
			if b, _ := s.r.Peek(len(ssePrefixData)); len(b) >= len(ssePrefixData) && bytes.Equal(b, ssePrefixData) {
				if haveData {
					frame.append([]byte{'\n'})
				}
				start := frame.size
				_, _ = s.r.Discard(len(ssePrefixData))
				if err := s.appendDataPayload(&frame); err != nil {
					return SSEByte{}, err
				}
				spans = append(spans, [2]int{start, frame.size})
				haveData = true
				continue
			}
		}
		line, lineErr := s.readLine()
		if len(line) > 0 {
			trimmed := bytes.TrimRight(line, "\r\n")
			switch {
			case len(trimmed) == 0:
				// blank line terminates the current event
				if haveData {
					return finish(), nil
				}
			case bytes.HasPrefix(trimmed, ssePrefixColon):
				// comment / keepalive
			case bytes.HasPrefix(trimmed, ssePrefixEvent):
				s.event = append(s.event[:0], bytes.TrimSpace(trimmed[len(ssePrefixEvent):])...)
			case bytes.HasPrefix(trimmed, ssePrefixData):
				// Peek could not classify it (end of stream mid-prefix):
				// take the slow path through the line buffer.
				if haveData {
					frame.append([]byte{'\n'})
				}
				start := frame.size
				frame.append(trimmed[len(ssePrefixData):])
				spans = append(spans, [2]int{start, frame.size})
				haveData = true
			}
		}
		if lineErr != nil {
			if lineErr == io.EOF && haveData {
				return finish(), nil
			}
			return SSEByte{}, lineErr
		}
	}
}

// appendDataPayload reads the remainder of a "data:"-prefixed line (the
// prefix itself is already discarded). Trimming happens once the frame
// is complete. EOF surfaces on the caller's next read.
func (s *SSEByteReader) appendDataPayload(frame *sseFrameBuffer) error {
	for {
		chunk, err := s.r.ReadSlice('\n')
		frame.append(chunk)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil && err != io.EOF {
			return err
		}
		break
	}
	return nil
}

// sseFrameBuffer first fills the previous frame's reusable backing array.
// Overflow uses fixed-size chunks, then one exact-sized consolidation at
// delivery. Cumulative payload allocation is roughly twice the new frame
// size, without rounding the retained buffer up to the next power of two.
// Scratch chunks belong to one Next call and are not retained or pooled.
type sseFrameBuffer struct {
	tail   []byte
	chunks [][]byte
	size   int
}

func (b *sseFrameBuffer) append(p []byte) {
	b.size += len(p)
	for len(p) > 0 {
		if len(b.tail) == cap(b.tail) {
			b.chunks = append(b.chunks, b.tail)
			b.tail = make([]byte, 0, 64*1024)
		}
		n := min(len(p), cap(b.tail)-len(b.tail))
		b.tail = append(b.tail, p[:n]...)
		p = p[n:]
	}
}

func (b *sseFrameBuffer) bytes() []byte {
	if len(b.chunks) == 0 {
		return b.tail
	}
	data := make([]byte, 0, b.size)
	for _, chunk := range b.chunks {
		data = append(data, chunk...)
	}
	return append(data, b.tail...)
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
