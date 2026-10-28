package middleware

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"reflect"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/relay/dto"
)

type routingMultipartPart struct {
	header textproto.MIMEHeader
	data   []byte
}

func routingMultipartParts(t *testing.T, contentType string, body []byte) []routingMultipartPart {
	t.Helper()
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var parts []routingMultipartPart
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			return parts
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, routingMultipartPart{part.Header, data})
	}
}

func TestDistributeMultipartModel(t *testing.T) {
	setupAutoRoutingDB(t)
	if err := model.InsertChannel(&model.Channel{Name: "multipart", Models: "whisper-1", Group: "default", Status: common.ChannelStatusEnabled, Weight: 1}); err != nil {
		t.Fatal(err)
	}
	if err := model.UpsertModelMeta(&model.ModelMeta{Model: "whisper-1", Tags: "stt"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, requested, group, limits string
		malformed                      bool
		status                         int
	}{
		{"auto", "myapis-auto", "default", "", false, http.StatusNoContent},
		{"restricted auto", "myapis-auto", "default", "whisper-1", false, http.StatusNoContent},
		{"explicit", "whisper-1", "default", "", false, http.StatusNoContent},
		{"denied", "myapis-auto", "default", "other-model", false, http.StatusForbidden},
		{"no candidates", "myapis-auto", "empty", "", false, http.StatusServiceUnavailable},
		{"malformed", "myapis-auto", "default", "", true, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var original bytes.Buffer
			writer := multipart.NewWriter(&original)
			parts := []routingMultipartPart{
				{textproto.MIMEHeader{"Content-Disposition": {`form-data; name="model"`}, "Content-Transfer-Encoding": {"quoted-printable"}}, []byte(tc.requested + "=20")},
				{textproto.MIMEHeader{"Content-Disposition": {`form-data; name="prompt"`}, "X-Review": {"preserved"}}, []byte("keep myapis-auto in this prompt")},
				{textproto.MIMEHeader{"Content-Disposition": {`form-data; name="model"; filename="myapis-auto.bin"`}, "Content-Type": {"application/octet-stream"}, "Content-Transfer-Encoding": {"quoted-printable"}}, []byte("myapis-auto=00=FF")},
				{textproto.MIMEHeader{"Content-Disposition": {`form-data; name="model"`}}, []byte("duplicate-model")},
				{textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="audio.wav"`}, "Content-Type": {"audio/wav"}}, append([]byte{0, 255, 13, 10}, bytes.Repeat([]byte("myapis-auto"), 1024)...)},
				{textproto.MIMEHeader{"Content-Disposition": {`form-data; name="stream"`}}, []byte("true")},
			}
			for _, part := range parts {
				target, err := writer.CreatePart(part.header)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := target.Write(part.data); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			body := original.Bytes()
			if tc.malformed {
				body = body[:len(body)-20]
			}
			contentType := writer.FormDataContentType()
			r := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader(body))
			r.Header.Set("Content-Type", contentType)
			r = r.WithContext(common.WithValues(r.Context(), common.CtxUsingGroup, tc.group, common.CtxTokenModelLimits, tc.limits))
			called := false
			handler := Distribute(dto.InboundOpenAI)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				cached := r.Context().Value(common.CtxRequestBody).([]byte)
				readBody, err := io.ReadAll(r.Body)
				if err != nil || !bytes.Equal(readBody, cached) || r.ContentLength != int64(len(cached)) {
					t.Fatalf("reader, length and replay body disagree: %v", err)
				}
				if r.Header.Get("Content-Type") != contentType || r.Context().Value(common.CtxRequestContentType) != contentType || r.Context().Value(common.CtxRequestModel) != "whisper-1" || r.Context().Value(common.CtxRequestedModel) != tc.requested || r.Context().Value(common.CtxRequestStream) != true {
					t.Fatal("routing metadata changed unexpectedly")
				}
				if tc.requested != "myapis-auto" && !bytes.Equal(cached, body) {
					t.Fatal("explicit-model multipart body was rewritten")
				}
				expected := routingMultipartParts(t, contentType, body)
				if tc.requested == "myapis-auto" {
					for _, i := range []int{0, 3} {
						expected[i].data = []byte("whisper-1")
						expected[i].header.Del("Content-Transfer-Encoding")
					}
				}
				if got := routingMultipartParts(t, contentType, cached); !reflect.DeepEqual(got, expected) {
					t.Fatal("multipart fields, files, headers or order changed unexpectedly")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			if rec.Code != tc.status || called != (tc.status == http.StatusNoContent) {
				t.Fatalf("status = %d, called = %v; want %d: %s", rec.Code, called, tc.status, rec.Body)
			}
		})
	}
}
