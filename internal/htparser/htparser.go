package htparser

import (
	"bytes"
	"errors"

	"github.com/valyala/bytebufferpool"
)

var (
	ErrBadProto       = errors.New("bad protocol")
	ErrMissingData    = errors.New("missing data")
	ErrHeaderOverflow = errors.New("header overflow")
)

const MaxHeaderSize = 80 * 1024

type state int

const (
	sDead state = iota
	sStartRes
	sResH
	sResHT
	sResHTT
	sResHTTP
	sResHttpMajor
	sResHttpDot
	sResHttpMinor
	sResHttpEnd
	sResFirstStatusCode
	sResStatusCode
	sResStatusStart
	sResStatus
	sResLineAlmostDone
	sHeaderFieldStart
	sHeaderField
	sHeaderValueDiscardWs
	sHeaderValueStart
	sHeaderValue
	sHeaderAlmostDone
	sHeadersAlmostDone
	sHeadersDone
)

type Header struct {
	Name  []byte
	Value []byte
}

type Parser struct {
	Version    []byte
	Status     []byte
	Reason     []byte
	Headers    []Header
	NumHeaders int

	ContentLengthVal int64
	Chunked          bool
	Upgrade          bool
	ConnectionClose  bool

	state        state
	buf          *bytebufferpool.ByteBuffer
	nread        int
	tokenStart   int
	currentField []byte
}

func NewParser() *Parser {
	return &Parser{
		Headers:          make([]Header, 10),
		state:            sStartRes,
		buf:              bytebufferpool.Get(),
		ContentLengthVal: -1,
		tokenStart:       -1,
	}
}

func (p *Parser) Reset() {
	p.Version = nil
	p.Status = nil
	p.Reason = nil
	for i := range p.Headers {
		p.Headers[i] = Header{}
	}
	p.NumHeaders = 0
	p.ContentLengthVal = -1
	p.Chunked = false
	p.Upgrade = false
	p.ConnectionClose = false
	p.state = sStartRes
	p.nread = 0
	p.tokenStart = -1
	p.currentField = nil
	if p.buf != nil {
		p.buf.Reset()
	}
}

func (p *Parser) Release() {
	if p.buf != nil {
		bytebufferpool.Put(p.buf)
		p.buf = nil
	}
}

func (p *Parser) copyBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	off := len(p.buf.B)
	p.buf.Write(b)
	return p.buf.B[off:]
}

func trimOWS(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\t') {
		b = b[:len(b)-1]
	}
	return b
}

func (p *Parser) addHeader(name, value []byte) {
	value = trimOWS(value)
	if p.NumHeaders == len(p.Headers) {
		newHeaders := make([]Header, len(p.Headers)*2)
		copy(newHeaders, p.Headers)
		p.Headers = newHeaders
	}
	p.Headers[p.NumHeaders] = Header{Name: name, Value: value}
	p.NumHeaders++

	if bytes.EqualFold(name, []byte("Content-Length")) {
		p.ContentLengthVal = parseContentLength(value)
	} else if bytes.EqualFold(name, []byte("Transfer-Encoding")) {
		if bytes.Contains(bytes.ToLower(value), []byte("chunked")) {
			p.Chunked = true
		}
	} else if bytes.EqualFold(name, []byte("Upgrade")) {
		p.Upgrade = true
	} else if bytes.EqualFold(name, []byte("Connection")) {
		if bytes.EqualFold(value, []byte("close")) {
			p.ConnectionClose = true
		}
	}
}

func parseContentLength(b []byte) int64 {
	var n int64
	for _, c := range b {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

func (p *Parser) FindHeader(name []byte) []byte {
	for i := 0; i < p.NumHeaders; i++ {
		h := p.Headers[i]
		if bytes.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return nil
}

func (p *Parser) ContentLength() int64 {
	return p.ContentLengthVal
}

var tokenChars = [256]bool{
	'!': true, '#': true, '$': true, '%': true, '&': true, '\'': true,
	'*': true, '+': true, '-': true, '.': true, '^': true, '_': true,
	'`': true, '|': true, '~': true,
}

func init() {
	for c := '0'; c <= '9'; c++ {
		tokenChars[c] = true
	}
	for c := 'A'; c <= 'Z'; c++ {
		tokenChars[c] = true
	}
	for c := 'a'; c <= 'z'; c++ {
		tokenChars[c] = true
	}
}

func isToken(c byte) bool {
	return tokenChars[c]
}

func (p *Parser) Parse(input []byte) (int, error) {
	p.nread = 0
	total := len(input)

	if p.state == sDead {
		return 0, ErrBadProto
	}

	idx := 0
	for idx < total {
		ch := input[idx]

		if p.state <= sHeadersDone {
			p.nread++
			if p.nread > MaxHeaderSize {
				p.state = sDead
				return 0, ErrHeaderOverflow
			}
		}

		switch p.state {
		case sStartRes:
			if ch == '\r' || ch == '\n' {
				idx++
				continue
			}
			if ch == 'H' {
				p.state = sResH
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sResH:
			if ch == 'T' {
				p.state = sResHT
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sResHT:
			if ch == 'T' {
				p.state = sResHTT
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sResHTT:
			if ch == 'P' {
				p.state = sResHTTP
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sResHTTP:
			if ch == '/' {
				p.state = sResHttpMajor
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sResHttpMajor:
			if ch >= '0' && ch <= '9' {
				p.Version = p.copyBytes(input[idx-5 : idx+1])
				p.state = sResHttpDot
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sResHttpDot:
			if ch == '.' {
				p.state = sResHttpMinor
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sResHttpMinor:
			if ch >= '0' && ch <= '9' {
				p.Version = p.copyBytes(input[idx-7 : idx+1])
				p.state = sResHttpEnd
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sResHttpEnd:
			if ch == ' ' {
				p.state = sResFirstStatusCode
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sResFirstStatusCode:
			if ch >= '0' && ch <= '9' {
				p.tokenStart = idx
				p.state = sResStatusCode
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sResStatusCode:
			if ch >= '0' && ch <= '9' {
				idx++
			} else if ch == ' ' {
				if idx-p.tokenStart != 3 {
					p.state = sDead
					return 0, ErrBadProto
				}
				p.Status = p.copyBytes(input[p.tokenStart:idx])
				p.tokenStart = -1
				p.state = sResStatusStart
				idx++
			} else if ch == '\r' || ch == '\n' {
				if idx-p.tokenStart != 3 {
					p.state = sDead
					return 0, ErrBadProto
				}
				p.Status = p.copyBytes(input[p.tokenStart:idx])
				p.tokenStart = -1
				p.state = sResStatusStart
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sResStatusStart:
			p.tokenStart = idx
			p.state = sResStatus

		case sResStatus:
			if ch == '\r' {
				if p.tokenStart >= 0 {
					p.Reason = p.copyBytes(input[p.tokenStart:idx])
					p.tokenStart = -1
				}
				p.state = sResLineAlmostDone
				idx++
			} else if ch == '\n' {
				if p.tokenStart >= 0 {
					p.Reason = p.copyBytes(input[p.tokenStart:idx])
					p.tokenStart = -1
				}
				p.state = sHeaderFieldStart
				idx++
			} else {
				idx++
			}

		case sResLineAlmostDone:
			if ch == '\n' {
				p.state = sHeaderFieldStart
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sHeaderFieldStart:
			if ch == '\r' {
				p.state = sHeadersAlmostDone
				idx++
			} else if ch == '\n' {
				p.state = sHeadersDone
				idx++
				p.finalizeConnectionState()
				return idx, nil
			} else if isToken(ch) {
				p.tokenStart = idx
				p.state = sHeaderField
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sHeaderField:
			if ch == ':' {
				if p.tokenStart >= 0 {
					p.currentField = p.copyBytes(input[p.tokenStart:idx])
					p.tokenStart = -1
				}
				p.state = sHeaderValueDiscardWs
				idx++
			} else if isToken(ch) {
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sHeaderValueDiscardWs:
			if ch == ' ' || ch == '\t' {
				idx++
			} else {
				p.tokenStart = idx
				p.state = sHeaderValue
			}

		case sHeaderValue:
			if ch == '\r' {
				if p.tokenStart >= 0 {
					val := p.copyBytes(input[p.tokenStart:idx])
					p.addHeader(p.currentField, val)
					p.tokenStart = -1
				}
				p.state = sHeaderAlmostDone
				idx++
			} else if ch == '\n' {
				if p.tokenStart >= 0 {
					val := p.copyBytes(input[p.tokenStart:idx])
					p.addHeader(p.currentField, val)
					p.tokenStart = -1
				}
				p.state = sHeaderFieldStart
				idx++
			} else {
				idx++
			}

		case sHeaderAlmostDone:
			if ch == '\n' {
				p.state = sHeaderFieldStart
				idx++
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		case sHeadersAlmostDone:
			if ch == '\n' {
				p.state = sHeadersDone
				idx++
				p.finalizeConnectionState()
				return idx, nil
			} else {
				p.state = sDead
				return 0, ErrBadProto
			}

		default:
			p.state = sDead
			return 0, ErrBadProto
		}
	}

	return 0, ErrMissingData
}

func (p *Parser) finalizeConnectionState() {
	if bytes.Equal(p.Version, []byte("HTTP/1.0")) {
		hasKeepAlive := false
		for i := 0; i < p.NumHeaders; i++ {
			h := p.Headers[i]
			if bytes.EqualFold(h.Name, []byte("Connection")) {
				if bytes.EqualFold(h.Value, []byte("keep-alive")) {
					hasKeepAlive = true
					break
				}
			}
		}
		if !hasKeepAlive {
			p.ConnectionClose = true
		}
	}
}
