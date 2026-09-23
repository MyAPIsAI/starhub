package channel

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestSSEByteReaderChunkBoundaryParity(t *testing.T) {
	var multiline strings.Builder
	for i := range 40 {
		fmt.Fprintf(&multiline, "data: \t%s %d \t\r\n", strings.Repeat("x", 35000), i)
	}
	multiline.WriteString("\r\n")
	cases := []string{
		multiline.String() + "data: small\n\n",
		"data: \n" + "data: " + strings.Repeat("z", 2<<20) + "\n" + "data: \t\n\n",
		"data: " + strings.Repeat("a", 1<<20) + "\n\ndata: " + strings.Repeat("b", 2<<20) + "\n\n",
		"data: " + strings.Repeat("z", 1<<20),
	}
	for pad := 65528; pad < 65538; pad++ {
		cases = append(cases, "data:"+strings.Repeat(" ", pad)+"\u2003payload\u2003\n\n")
		cases = append(cases, "data: payload"+strings.Repeat(" ", pad)+"\u2003\n\n")
	}
	for i, input := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			legacy := NewSSEReader(strings.NewReader(input))
			reader := NewSSEByteReader(strings.NewReader(input))
			for {
				want, wantErr := legacy.Next()
				got, gotErr := reader.Next()
				if !errors.Is(gotErr, wantErr) || string(got.Data) != want.Data || string(got.Event) != want.Event {
					t.Fatalf("framing differs: got data=%d err=%v, want data=%d err=%v", len(got.Data), gotErr, len(want.Data), wantErr)
				}
				if wantErr != nil {
					break
				}
			}
		})
	}
}

func TestSSEByteReaderReadFailure(t *testing.T) {
	failure := errors.New("upstream disconnected")
	reader := NewSSEByteReader(io.MultiReader(
		strings.NewReader("data: "+strings.Repeat("x", 200*1024)), sseFailedReader{failure}))
	frame, err := reader.Next()
	if !errors.Is(err, failure) || len(frame.Data) != 0 {
		t.Fatalf("incomplete event delivered: len=%d err=%v", len(frame.Data), err)
	}
}

func TestSSEByteReaderRetainedCapacity(t *testing.T) {
	const payloadSize = 1092364
	reader := NewSSEByteReader(strings.NewReader("data: " + strings.Repeat("x", payloadSize) + "\n\n"))
	frame, err := reader.Next()
	if err != nil || len(frame.Data) != payloadSize {
		t.Fatalf("frame len=%d err=%v", len(frame.Data), err)
	}
	if cap(frame.Data) > payloadSize+64*1024 {
		t.Fatalf("oversized retained frame: payload=%d capacity=%d", payloadSize, cap(frame.Data))
	}
}

type sseFailedReader struct{ err error }

func (r sseFailedReader) Read([]byte) (int, error) { return 0, r.err }

func BenchmarkSSEByteReaderWorkloads(b *testing.B) {
	for _, tc := range []struct {
		name                string
		size, frames, lines int
	}{
		{"small", 4096, 1, 1},
		{"boundary", 64 * 1024, 1, 1},
		{"p7", 1092364, 1, 1},
		{"p7_four_frames", 1092364, 4, 1},
		{"p7_multiline", 1092364, 1, 32},
		{"large", 8 << 20, 1, 1},
	} {
		b.Run(tc.name, func(b *testing.B) {
			var input strings.Builder
			line := strings.Repeat("A", tc.size/tc.lines)
			for range tc.frames {
				for range tc.lines {
					fmt.Fprintf(&input, "data: %s\n", line)
				}
				input.WriteString("\n")
			}
			raw := []byte(input.String())
			want := len(line)*tc.lines + tc.lines - 1
			b.ReportAllocs()
			b.SetBytes(int64(len(raw)))
			b.ResetTimer()
			for b.Loop() {
				reader := NewSSEByteReader(bytes.NewReader(raw))
				for range tc.frames {
					frame, err := reader.Next()
					if err != nil || len(frame.Data) != want {
						b.Fatalf("size=%d want=%d err=%v", len(frame.Data), want, err)
					}
				}
			}
		})
	}
}

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
