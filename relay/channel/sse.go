package channel

import (
	"bufio"
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
