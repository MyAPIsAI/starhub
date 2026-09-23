package channel

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func BenchmarkSSEByteReaderImage(b *testing.B) {
	input := []byte("data: " + strings.Repeat("A", 1<<20) + "\n\n")
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for b.Loop() {
		reader := NewSSEByteReader(bytes.NewReader(input))
		frame, err := reader.Next()
		if err != nil || len(frame.Data) != 1<<20 {
			b.Fatalf("frame size=%d err=%v", len(frame.Data), err)
		}
	}
}

func readAllSSEBytes(t *testing.T, input string) []SSEByte {
	t.Helper()
	reader := NewSSEByteReader(strings.NewReader(input))
	var out []SSEByte
	for {
		line, err := reader.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Snapshot the buffers: the reuse contract makes the previous
		// frame's slices invalid after the next Next call.
		out = append(out, SSEByte{
			Event: append([]byte(nil), line.Event...),
			Data:  append([]byte(nil), line.Data...),
		})
	}
}

func TestSSEByteReaderFraming(t *testing.T) {
	input := "event: alpha\ndata: one\n\n" +
		"data: two\n\n" +
		"event: beta\r\ndata: three\r\n\r\n" +
		": keepalive\n\n" +
		"data: line1\ndata: line2\n\n"
	got := readAllSSEBytes(t, input)
	want := []SSEByte{
		{Event: []byte("alpha"), Data: []byte("one")},
		{Data: []byte("two")},
		{Event: []byte("beta"), Data: []byte("three")},
		// The beta event name was consumed and reset at delivery (legacy
		// SSEReader semantics); this frame carries data only.
		{Data: []byte("line1\nline2")},
	}
	if len(got) != len(want) {
		t.Fatalf("frame count = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if !bytes.Equal(got[i].Event, want[i].Event) || !bytes.Equal(got[i].Data, want[i].Data) {
			t.Fatalf("frame %d = (%q, %q), want (%q, %q)", i, got[i].Event, got[i].Data, want[i].Event, want[i].Data)
		}
	}
}

func TestSSEByteReaderEOFWithPendingData(t *testing.T) {
	got := readAllSSEBytes(t, "data: [DONE]") // no trailing newline
	if len(got) != 1 || string(got[0].Data) != "[DONE]" {
		t.Fatalf("pending EOF frame = %+v", got)
	}
}

func TestSSEByteReaderCleanEOF(t *testing.T) {
	reader := NewSSEByteReader(strings.NewReader("data: x\n\n"))
	if _, err := reader.Next(); err != nil {
		t.Fatalf("first frame: %v", err)
	}
	if _, err := reader.Next(); err != io.EOF {
		t.Fatalf("second frame error = %v, want io.EOF", err)
	}
}

func TestSSEByteReaderLongLines(t *testing.T) {
	// A data line far beyond the 64KB bufio buffer exercises the
	// ErrBufferFull accumulation loop.
	big := strings.Repeat("x", 300*1024)
	input := "event: image_generation.completed\ndata: " + big + "\n\n"
	got := readAllSSEBytes(t, input)
	if len(got) != 1 || string(got[0].Data) != big || string(got[0].Event) != "image_generation.completed" {
		t.Fatalf("long line frame mangled: data len %d", len(got[0].Data))
	}
}

func TestSSEByteReaderBufferReuse(t *testing.T) {
	// The single-owner contract: after the second Next the first frame's
	// slices are overwritten. Two megabyte-ish frames must share memory,
	// so the reader allocates the payload capacity once.
	big1 := strings.Repeat("a", 200*1024)
	big2 := strings.Repeat("b", 200*1024)
	reader := NewSSEByteReader(strings.NewReader(
		"data: " + big1 + "\n\ndata: " + big2 + "\n\n"))

	first, err := reader.Next()
	if err != nil || string(first.Data) != big1 {
		t.Fatalf("first frame: err=%v len=%d", err, len(first.Data))
	}
	second, err := reader.Next()
	if err != nil || string(second.Data) != big2 {
		t.Fatalf("second frame: err=%v len=%d", err, len(second.Data))
	}
	// Same backing array: the reuse actually happened.
	if &first.Data[:1][0] != &second.Data[:1][0] {
		t.Fatalf("data buffers were not reused between frames")
	}
	if string(first.Data[:8]) != big2[:8] {
		t.Fatalf("first frame was not overwritten by the second")
	}
}
