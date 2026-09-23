package controller

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestReadBoundedImageBody(t *testing.T) {
	payload := bytes.Repeat([]byte("image"), 15000)
	for _, size := range []int{0, 1, 32768, len(payload)} {
		for _, declared := range []int64{-1, 0, 1, 32768, int64(size), int64(size + 10)} {
			for _, limit := range []int64{0, 1, 32768, int64(len(payload) + 1)} {
				t.Run(fmt.Sprintf("size=%d/declared=%d/limit=%d", size, declared, limit), func(t *testing.T) {
					resp := &http.Response{Body: io.NopCloser(bytes.NewReader(payload[:size])), ContentLength: declared}
					got := readBoundedBody(resp, limit)
					if !bytes.Equal(got, payload[:min(int64(size), limit)]) {
						t.Fatalf("bounded body changed: got=%d want=%d", len(got), min(int64(size), limit))
					}
				})
			}
		}
	}
}

func BenchmarkReadBoundedImageBody(b *testing.B) {
	payload := bytes.Repeat([]byte("A"), 1<<20)
	for _, declared := range []int64{-1, int64(len(payload))} {
		b.Run(fmt.Sprintf("contentLength=%d", declared), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			for b.Loop() {
				resp := &http.Response{Body: io.NopCloser(bytes.NewReader(payload)), ContentLength: declared}
				if got := readBoundedBody(resp, maxModeResponseBytes); !bytes.Equal(got, payload) {
					b.Fatal("response changed")
				}
			}
		})
	}
}
