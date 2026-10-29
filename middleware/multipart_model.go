package middleware

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"strings"
)

// rewriteMultipartModel replaces model fields while streaming file parts
// into the new replay body. The original boundary, file headers and part
// order remain intact; uploads never need separate in-memory buffers.
func rewriteMultipartModel(contentType string, body []byte, modelName string) ([]byte, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	output.Grow(len(body))
	writer := multipart.NewWriter(&output)
	if err := writer.SetBoundary(params["boundary"]); err != nil {
		return nil, err
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		// NextRawPart preserves transfer-encoded file bytes and headers.
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		var source io.Reader = part
		if part.FormName() == "model" && part.FileName() == "" {
			// The replacement value is plain text, regardless of the old encoding.
			part.Header.Del("Content-Transfer-Encoding")
			source = strings.NewReader(modelName)
		}
		target, err := writer.CreatePart(part.Header)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(target, source); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// RewriteMultipartModel preserves multipart parts while replacing the model field.
func RewriteMultipartModel(contentType string, body []byte, modelName string) ([]byte, error) {
	return rewriteMultipartModel(contentType, body, modelName)
}
