package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type wsPeer struct {
	conn    net.Conn
	reader  *bufio.Reader
	client  bool
	writeMu sync.Mutex
}

func websocketAccept(key string) string {
	h := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

func headerHasToken(h http.Header, name, token string) bool {
	for _, value := range h.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	return false
}

func wsClientHandshake(ctx context.Context, conn net.Conn, path, host string) (*wsPeer, error) {
	var rawKey [16]byte
	if _, err := rand.Read(rawKey[:]); err != nil {
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(rawKey[:])
	deadline := time.Now().Add(8 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	defer conn.SetDeadline(time.Time{})

	if _, err := fmt.Fprintf(conn,
		"GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\nUser-Agent: Hashshashin/0.3\r\n\r\n",
		path, host, key); err != nil {
		return nil, err
	}

	r := bufio.NewReader(conn)
	resp, err := http.ReadResponse(r, &http.Request{Method: http.MethodGet})
	if err != nil {
		return nil, fmt.Errorf("websocket response: %w", err)
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if resp.StatusCode != http.StatusSwitchingProtocols || !headerHasToken(resp.Header, "Upgrade", "websocket") || !headerHasToken(resp.Header, "Connection", "upgrade") {
		return nil, fmt.Errorf("websocket upgrade rejected: %s", resp.Status)
	}
	if resp.Header.Get("Sec-WebSocket-Accept") != websocketAccept(key) {
		return nil, fmt.Errorf("websocket accept hash mismatch")
	}
	return &wsPeer{conn: conn, reader: r, client: true}, nil
}

func wsServerHandshake(conn net.Conn, path string) (*wsPeer, error) {
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	defer conn.SetDeadline(time.Time{})
	r := bufio.NewReader(conn)
	req, err := http.ReadRequest(r)
	if err != nil {
		return nil, fmt.Errorf("websocket request: %w", err)
	}
	if req.Body != nil {
		defer req.Body.Close()
	}
	if req.Method != http.MethodGet || req.URL == nil || req.URL.Path != path {
		return nil, fmt.Errorf("websocket request path/method rejected")
	}
	if !headerHasToken(req.Header, "Upgrade", "websocket") || !headerHasToken(req.Header, "Connection", "upgrade") || req.Header.Get("Sec-WebSocket-Version") != "13" {
		return nil, fmt.Errorf("invalid websocket upgrade headers")
	}
	key := strings.TrimSpace(req.Header.Get("Sec-WebSocket-Key"))
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decoded) != 16 {
		return nil, fmt.Errorf("invalid websocket key")
	}
	if _, err := fmt.Fprintf(conn,
		"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n",
		websocketAccept(key)); err != nil {
		return nil, err
	}
	return &wsPeer{conn: conn, reader: r, client: false}, nil
}

func (w *wsPeer) readBinary(buf []byte) (int, error) {
	for {
		var h [2]byte
		if _, err := io.ReadFull(w.reader, h[:]); err != nil {
			return 0, err
		}
		if h[0]&0x70 != 0 {
			return 0, fmt.Errorf("websocket RSV bits are unsupported")
		}
		fin := h[0]&0x80 != 0
		opcode := h[0] & 0x0f
		masked := h[1]&0x80 != 0
		length := uint64(h[1] & 0x7f)
		if length == 126 {
			var ext [2]byte
			if _, err := io.ReadFull(w.reader, ext[:]); err != nil { return 0, err }
			length = uint64(binary.BigEndian.Uint16(ext[:]))
		} else if length == 127 {
			var ext [8]byte
			if _, err := io.ReadFull(w.reader, ext[:]); err != nil { return 0, err }
			length = binary.BigEndian.Uint64(ext[:])
			if length>>63 != 0 { return 0, fmt.Errorf("invalid websocket length") }
		}
		control := opcode >= 0x8
		if control && (!fin || length > 125) {
			return 0, fmt.Errorf("invalid websocket control frame")
		}
		// RFC6455: client-to-server frames are masked, server-to-client frames are not.
		if (!w.client && !masked) || (w.client && masked) {
			return 0, fmt.Errorf("invalid websocket masking direction")
		}
		if length > maxCarrierFrame || length > uint64(len(buf)) {
			return 0, fmt.Errorf("websocket frame too large: %d", length)
		}
		var mask [4]byte
		if masked {
			if _, err := io.ReadFull(w.reader, mask[:]); err != nil { return 0, err }
		}
		payload := buf[:int(length)]
		if _, err := io.ReadFull(w.reader, payload); err != nil { return 0, err }
		if masked {
			for i := range payload { payload[i] ^= mask[i&3] }
		}
		switch opcode {
		case 0x2: // binary
			if !fin { return 0, fmt.Errorf("fragmented websocket data frames are unsupported") }
			return len(payload), nil
		case 0x8: // close
			_ = w.writeFrame(0x8, payload)
			return 0, io.EOF
		case 0x9: // ping
			if err := w.writeFrame(0xA, payload); err != nil { return 0, err }
			continue
		case 0xA: // pong
			continue
		default:
			return 0, fmt.Errorf("unsupported websocket opcode 0x%x", opcode)
		}
	}
}

func (w *wsPeer) writeBinary(p []byte) error { return w.writeFrame(0x2, p) }

func (w *wsPeer) writeFrame(opcode byte, p []byte) error {
	if len(p) > maxCarrierFrame {
		return fmt.Errorf("websocket frame too large: %d", len(p))
	}
	w.writeMu.Lock()
	defer w.writeMu.Unlock()

	masked := w.client
	header := make([]byte, 0, 14)
	header = append(header, 0x80|(opcode&0x0f))
	maskBit := byte(0)
	if masked { maskBit = 0x80 }
	switch {
	case len(p) < 126:
		header = append(header, maskBit|byte(len(p)))
	case len(p) <= 65535:
		header = append(header, maskBit|126, 0, 0)
		binary.BigEndian.PutUint16(header[len(header)-2:], uint16(len(p)))
	default:
		header = append(header, maskBit|127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(len(p)))
		header = append(header, ext[:]...)
	}
	payload := p
	if masked {
		var mask [4]byte
		if _, err := rand.Read(mask[:]); err != nil { return err }
		header = append(header, mask[:]...)
		payload = make([]byte, len(p))
		for i := range p { payload[i] = p[i] ^ mask[i&3] }
	}
	if err := writeAll(w.conn, header); err != nil { return err }
	return writeAll(w.conn, payload)
}

func (w *wsPeer) Close() error { return w.conn.Close() }
