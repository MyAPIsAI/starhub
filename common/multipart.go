package common

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
)

// maxMultipartFieldBytes bounds one non-file form field.
const maxMultipartFieldBytes = 1 << 20

// MultipartFile is one uploaded file part of a multipart body.
type MultipartFile struct {
	FieldName   string
	FileName    string
	ContentType string
	Data        []byte
}

// MultipartForm is a parsed, replayable multipart body: every attempt of
// the relay loop re-sends the cached raw bytes, and billing paths read the
// parsed fields (P3.1: multipart parsing must not consume the request).
type MultipartForm struct {
	Values map[string]string
	Files  []MultipartFile
}

// Value returns a form field ("" when absent).
func (f *MultipartForm) Value(name string) string {
	if f == nil {
		return ""
	}
	return f.Values[name]
}

// File returns the first file of the named field (nil when absent).
func (f *MultipartForm) File(name string) *MultipartFile {
	if f == nil {
		return nil
	}
	for i := range f.Files {
		if f.Files[i].FieldName == name {
			return &f.Files[i]
		}
	}
	return nil
}

// FileBytes returns the total uploaded bytes (the STT billing heuristic
// input).
func (f *MultipartForm) FileBytes() int64 {
	if f == nil {
		return 0
	}
	var total int64
	for i := range f.Files {
		total += int64(len(f.Files[i].Data))
	}
	return total
}

// ParseMultipartForm parses a cached multipart body (raw bytes plus the
// original Content-Type header carrying the boundary). Part sizes are
// bounded by the distributor's body limit.
func ParseMultipartForm(contentType string, body []byte) (*MultipartForm, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, errors.New("invalid multipart Content-Type: " + err.Error())
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, errors.New("Content-Type is not multipart")
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	form := &MultipartForm{Values: map[string]string{}}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("malformed multipart body: " + err.Error())
		}
		fieldName := part.FormName()
		if fieldName == "" {
			continue
		}
		if part.FileName() == "" {
			value, err := io.ReadAll(io.LimitReader(part, maxMultipartFieldBytes))
			part.Close()
			if err != nil {
				return nil, errors.New("malformed multipart body: " + err.Error())
			}
			if _, exists := form.Values[fieldName]; !exists {
				form.Values[fieldName] = strings.TrimSpace(string(value))
			}
			continue
		}
		fileBytes, err := io.ReadAll(part)
		part.Close()
		if err != nil {
			return nil, errors.New("malformed multipart body: " + err.Error())
		}
		form.Files = append(form.Files, MultipartFile{
			FieldName:   fieldName,
			FileName:    part.FileName(),
			ContentType: part.Header.Get("Content-Type"),
			Data:        fileBytes,
		})
	}
	return form, nil
}

// BuildMultipartForm re-encodes a parsed form into a sendable body (used
// when an adaptor must reassemble a multipart request after edits). The
// returned Content-Type carries the generated boundary.
func BuildMultipartForm(form *MultipartForm) (body []byte, contentType string, err error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if form != nil {
		for name, value := range form.Values {
			if err := writer.WriteField(name, value); err != nil {
				return nil, "", err
			}
		}
		for _, file := range form.Files {
			header := make(textproto.MIMEHeader)
			header.Set("Content-Disposition",
				`form-data; name="`+escapeQuote(file.FieldName)+`"; filename="`+escapeQuote(file.FileName)+`"`)
			if file.ContentType != "" {
				header.Set("Content-Type", file.ContentType)
			}
			part, err := writer.CreatePart(header)
			if err != nil {
				return nil, "", err
			}
			if _, err := part.Write(file.Data); err != nil {
				return nil, "", err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), writer.FormDataContentType(), nil
}

func escapeQuote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// AssertJSONContentType reports whether the header names a JSON body.
func AssertJSONContentType(header string) bool {
	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil {
		return strings.Contains(strings.ToLower(header), "json")
	}
	return strings.HasSuffix(strings.ToLower(mediaType), "json")
}
