package common

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"testing"
)

// buildMultipartBody encodes a small form for the metadata-parse tests.
func buildMultipartBody(t *testing.T, fields map[string]string, files map[string][]byte) (body []byte, contentType string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	for name, data := range files {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, name, name+".bin"))
		header.Set("Content-Type", "application/octet-stream")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), writer.FormDataContentType()
}

// TestParseMultipartFormMetaParity cross-checks the metadata-only parse
// against the materializing one: values, file presence, and byte counts
// must agree (P7.1 replaced the buffering, not the semantics).
func TestParseMultipartFormMetaParity(t *testing.T) {
	image := make([]byte, 300*1024) // > bufio buffer: exercises the skip loop
	for i := range image {
		image[i] = byte(i)
	}
	body, contentType := buildMultipartBody(t,
		map[string]string{"model": "gpt-image-1", "prompt": "a cat", "stream": "true"},
		map[string][]byte{"image": image, "mask": []byte("small")})

	full, err := ParseMultipartForm(contentType, body)
	if err != nil {
		t.Fatalf("full parse: %v", err)
	}
	meta, err := ParseMultipartFormMeta(contentType, body)
	if err != nil {
		t.Fatalf("meta parse: %v", err)
	}

	for name, want := range map[string]string{"model": "gpt-image-1", "prompt": "a cat", "stream": "true"} {
		if got := meta.Value(name); got != want {
			t.Fatalf("Value(%s) = %q, want %q", name, got, want)
		}
		if got := full.Value(name); got != want {
			t.Fatalf("full Value(%s) = %q, want %q", name, got, want)
		}
	}
	if !meta.HasFile("image") || !meta.HasFile("mask") {
		t.Fatalf("HasFile lost a file: %+v", meta.Files)
	}
	if meta.HasFile("nope") {
		t.Fatalf("HasFile invented a file")
	}
	if len(full.Files) != len(meta.Files) {
		t.Fatalf("file count: full=%d meta=%d", len(full.Files), len(meta.Files))
	}
	for i := range full.Files {
		if full.Files[i].FieldName != meta.Files[i].FieldName ||
			full.Files[i].FileName != meta.Files[i].FileName ||
			full.Files[i].ContentType != meta.Files[i].ContentType ||
			int64(len(full.Files[i].Data)) != meta.Files[i].Bytes {
			t.Fatalf("file %d mismatch: full=%+v meta=%+v", i, full.Files[i], meta.Files[i])
		}
	}
	if got, want := meta.FileBytes(), full.FileBytes(); got != want {
		t.Fatalf("FileBytes = %d, want %d", got, want)
	}
}

func TestParseMultipartFormMetaNoFiles(t *testing.T) {
	body, contentType := buildMultipartBody(t, map[string]string{"model": "whisper-1"}, nil)
	meta, err := ParseMultipartFormMeta(contentType, body)
	if err != nil {
		t.Fatalf("meta parse: %v", err)
	}
	if meta.Value("model") != "whisper-1" || len(meta.Files) != 0 || meta.FileBytes() != 0 {
		t.Fatalf("meta = %+v", meta)
	}
	if meta.HasFile("file") {
		t.Fatalf("HasFile on a files-less form must be false")
	}
}

func TestParseMultipartFormMetaErrors(t *testing.T) {
	if _, err := ParseMultipartFormMeta("application/json", []byte("{}")); err == nil {
		t.Fatalf("non-multipart content type must error")
	}
	if _, err := ParseMultipartFormMeta("multipart/form-data; boundary=x", []byte("garbage")); err == nil {
		t.Fatalf("malformed body must error")
	}
	var nilMeta *MultipartFormMeta
	if nilMeta.Value("x") != "" || nilMeta.HasFile("x") || nilMeta.FileBytes() != 0 {
		t.Fatalf("nil receiver helpers must be inert")
	}
}
