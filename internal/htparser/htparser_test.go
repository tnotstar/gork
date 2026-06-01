package htparser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParserSuccess(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectVer     string
		expectStatus  string
		expectReason  string
		expectHeaders map[string]string
		expectCL      int64
		expectChunked bool
		expectClose   bool
		expectUpgrade bool
	}{
		{
			name:         "Standard OK response",
			input:        "HTTP/1.1 200 OK\r\nContent-Length: 12\r\nContent-Type: text/plain\r\n\r\n",
			expectVer:    "HTTP/1.1",
			expectStatus: "200",
			expectReason: "OK",
			expectHeaders: map[string]string{
				"Content-Length": "12",
				"Content-Type":   "text/plain",
			},
			expectCL:      12,
			expectChunked: false,
			expectClose:   false,
			expectUpgrade: false,
		},
		{
			name:         "HTTP/1.0 default close response",
			input:        "HTTP/1.0 404 Not Found\r\n\r\n",
			expectVer:    "HTTP/1.0",
			expectStatus: "404",
			expectReason: "Not Found",
			expectHeaders: map[string]string{},
			expectCL:      -1,
			expectChunked: false,
			expectClose:   true,
			expectUpgrade: false,
		},
		{
			name:         "HTTP/1.0 explicit keep-alive response",
			input:        "HTTP/1.0 200 OK\r\nConnection: keep-alive\r\n\r\n",
			expectVer:    "HTTP/1.0",
			expectStatus: "200",
			expectReason: "OK",
			expectHeaders: map[string]string{
				"Connection": "keep-alive",
			},
			expectCL:      -1,
			expectChunked: false,
			expectClose:   false,
			expectUpgrade: false,
		},
		{
			name:         "HTTP/1.1 explicit Connection close",
			input:        "HTTP/1.1 200 OK\r\nConnection: close\r\n\r\n",
			expectVer:    "HTTP/1.1",
			expectStatus: "200",
			expectReason: "OK",
			expectHeaders: map[string]string{
				"Connection": "close",
			},
			expectCL:      -1,
			expectChunked: false,
			expectClose:   true,
			expectUpgrade: false,
		},
		{
			name:         "Chunked transfer encoding response",
			input:        "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nConnection: keep-alive\r\n\r\n",
			expectVer:    "HTTP/1.1",
			expectStatus: "200",
			expectReason: "OK",
			expectHeaders: map[string]string{
				"Transfer-Encoding": "chunked",
				"Connection":        "keep-alive",
			},
			expectCL:      -1,
			expectChunked: true,
			expectClose:   false,
			expectUpgrade: false,
		},
		{
			name:         "Empty reason phrase response",
			input:        "HTTP/1.1 204 \r\nContent-Length: 0\r\n\r\n",
			expectVer:    "HTTP/1.1",
			expectStatus: "204",
			expectReason: "",
			expectHeaders: map[string]string{
				"Content-Length": "0",
			},
			expectCL:      0,
			expectChunked: false,
			expectClose:   false,
			expectUpgrade: false,
		},
		{
			name:         "Spaces in headers",
			input:        "HTTP/1.1 200 OK\r\nHeader:   value \r\n\r\n",
			expectVer:    "HTTP/1.1",
			expectStatus: "200",
			expectReason: "OK",
			expectHeaders: map[string]string{
				"Header": "value",
			},
			expectCL:      -1,
			expectChunked: false,
			expectClose:   false,
			expectUpgrade: false,
		},
		{
			name:         "Tabs and mixed spacing in header value",
			input:        "HTTP/1.1 200 OK\r\nHeader:\t  value \t\t \r\n\r\n",
			expectVer:    "HTTP/1.1",
			expectStatus: "200",
			expectReason: "OK",
			expectHeaders: map[string]string{
				"Header": "value",
			},
			expectCL:      -1,
			expectChunked: false,
			expectClose:   false,
			expectUpgrade: false,
		},
		{
			name:         "HTTP/1.1 Upgrade websocket",
			input:        "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n",
			expectVer:    "HTTP/1.1",
			expectStatus: "101",
			expectReason: "Switching Protocols",
			expectHeaders: map[string]string{
				"Upgrade":    "websocket",
				"Connection": "Upgrade",
			},
			expectCL:      -1,
			expectChunked: false,
			expectClose:   false,
			expectUpgrade: true,
		},
		{
			name:         "Pure LF line endings",
			input:        "HTTP/1.1 200 OK\nHost: localhost\nContent-Length: 0\n\n",
			expectVer:    "HTTP/1.1",
			expectStatus: "200",
			expectReason: "OK",
			expectHeaders: map[string]string{
				"Host":           "localhost",
				"Content-Length": "0",
			},
			expectCL:      0,
			expectChunked: false,
			expectClose:   false,
			expectUpgrade: false,
		},
		{
			name:         "Invalid characters in Content-Length",
			input:        "HTTP/1.1 200 OK\r\nContent-Length: 12a3\r\n\r\n",
			expectVer:    "HTTP/1.1",
			expectStatus: "200",
			expectReason: "OK",
			expectHeaders: map[string]string{
				"Content-Length": "12a3",
			},
			expectCL:      -1,
			expectChunked: false,
			expectClose:   false,
			expectUpgrade: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			defer p.Release()

			n, err := p.Parse([]byte(tt.input))
			assert.NoError(t, err)
			assert.Equal(t, len(tt.input), n)

			assert.Equal(t, tt.expectVer, string(p.Version))
			assert.Equal(t, tt.expectStatus, string(p.Status))
			assert.Equal(t, tt.expectReason, string(p.Reason))
			assert.Equal(t, tt.expectCL, p.ContentLength())
			assert.Equal(t, tt.expectChunked, p.Chunked)
			assert.Equal(t, tt.expectClose, p.ConnectionClose)
			assert.Equal(t, tt.expectUpgrade, p.Upgrade)

			for k, v := range tt.expectHeaders {
				val := p.FindHeader([]byte(k))
				assert.NotNil(t, val)
				assert.Equal(t, v, string(val))
			}
		})
	}
}

func TestParserErrors(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError error
	}{
		{
			name:        "Bad protocol prefix 1",
			input:       "HTP/1.1 200 OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Bad protocol prefix 2",
			input:       "FTP/1.1 200 OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Bad protocol letter 2",
			input:       "HUTP/1.1 200 OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Bad protocol letter 3",
			input:       "HTUP/1.1 200 OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Bad protocol letter 4",
			input:       "HTTU/1.1 200 OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Bad protocol separator",
			input:       "HTTP.1.1 200 OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Bad HTTP major version (non-digit)",
			input:       "HTTP/A.1 200 OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Bad HTTP dot separator",
			input:       "HTTP/1?1 200 OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Bad HTTP minor version (non-digit)",
			input:       "HTTP/1.A 200 OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Bad HTTP end (missing space)",
			input:       "HTTP/1.1A 200 OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Bad status code digits count",
			input:       "HTTP/1.1 20 OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Bad status code characters",
			input:       "HTTP/1.1 20A OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Missing space after status code",
			input:       "HTTP/1.1 200OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Status code exceeding 999",
			input:       "HTTP/1.1 1000 OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Status code first digit non-numeric",
			input:       "HTTP/1.1 A00 OK\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Invalid char in header field",
			input:       "HTTP/1.1 200 OK\r\nHeader@Field: value\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Missing LF after CR in status line",
			input:       "HTTP/1.1 200 OK\rHost: localhost\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Missing LF after CR in header line",
			input:       "HTTP/1.1 200 OK\r\nHeader: value\rHost: localhost\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Missing LF after CR in headers end",
			input:       "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\rHost: localhost",
			expectError: ErrBadProto,
		},
		{
			name:        "Invalid char in header field start",
			input:       "HTTP/1.1 200 OK\r\n@Header: value\r\n\r\n",
			expectError: ErrBadProto,
		},
		{
			name:        "Incomplete headers",
			input:       "HTTP/1.1 200 OK\r\nContent-Length: 12",
			expectError: ErrMissingData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser()
			defer p.Release()

			_, err := p.Parse([]byte(tt.input))
			assert.ErrorIs(t, err, tt.expectError)
		})
	}
}

func TestParserHeaderOverflow(t *testing.T) {
	p := NewParser()
	defer p.Release()

	largeHeader := make([]byte, MaxHeaderSize+100)
	copy(largeHeader[:15], []byte("HTTP/1.1 200 OK\r\n"))
	for i := 15; i < len(largeHeader)-4; i++ {
		largeHeader[i] = 'a'
	}
	copy(largeHeader[len(largeHeader)-4:], []byte("\r\n\r\n"))

	_, err := p.Parse(largeHeader)
	assert.ErrorIs(t, err, ErrHeaderOverflow)
}

func TestParserHeaderExpansion(t *testing.T) {
	p := NewParser()
	defer p.Release()

	input := "HTTP/1.1 200 OK\r\n" +
		"H1: v1\r\n" +
		"H2: v2\r\n" +
		"H3: v3\r\n" +
		"H4: v4\r\n" +
		"H5: v5\r\n" +
		"H6: v6\r\n" +
		"H7: v7\r\n" +
		"H8: v8\r\n" +
		"H9: v9\r\n" +
		"H10: v10\r\n" +
		"H11: v11\r\n" +
		"H12: v12\r\n\r\n"

	n, err := p.Parse([]byte(input))
	assert.NoError(t, err)
	assert.Equal(t, len(input), n)
	assert.Equal(t, 12, p.NumHeaders)
	assert.True(t, len(p.Headers) >= 12)

	assert.Equal(t, "v1", string(p.FindHeader([]byte("H1"))))
	assert.Equal(t, "v12", string(p.FindHeader([]byte("H12"))))
}

func TestParserResetAndRelease(t *testing.T) {
	p := NewParser()

	input := "HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\n"
	_, err := p.Parse([]byte(input))
	assert.NoError(t, err)

	assert.Equal(t, "HTTP/1.1", string(p.Version))
	assert.Equal(t, "200", string(p.Status))
	assert.Equal(t, int64(10), p.ContentLength())

	p.Reset()
	assert.Nil(t, p.Version)
	assert.Nil(t, p.Status)
	assert.Equal(t, int64(-1), p.ContentLength())
	assert.Equal(t, 0, p.NumHeaders)

	_, err = p.Parse([]byte(input))
	assert.NoError(t, err)
	assert.Equal(t, "HTTP/1.1", string(p.Version))

	p.Release()
	assert.Nil(t, p.buf)

	p.Reset()
}

func TestParserFindHeaderNonExistent(t *testing.T) {
	p := NewParser()
	defer p.Release()

	input := "HTTP/1.1 200 OK\r\nHost: localhost\r\n\r\n"
	_, err := p.Parse([]byte(input))
	assert.NoError(t, err)

	assert.Nil(t, p.FindHeader([]byte("X-Non-Existent")))
}
