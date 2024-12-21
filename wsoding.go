package wsoding

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"syscall"
	"unsafe"

	"github.com/mdlayher/socket"
)

const chunkSize int = 1024

var dummyErr = fmt.Errorf("dummy error")
var extraDummy = fmt.Errorf("extra")

type WS struct {
	sock   *socket.Conn
	Debug  bool
	Client bool
}

func (ws *WS) Close() error {
	// Base on the ideas from https://blog.netherlabs.nl/articles/2009/01/18/the-ultimate-so_linger-page-or-why-is-my-tcp-not-reliable
	// Informing the OS that we are not planning to send anything anymore
	if err := ws.sock.Shutdown(syscall.SHUT_WR); err != nil {
		return err
	}
	// Depleting input before closing socket, so the OS does not send RST just because we have some input pending on close
	buffer := make([]byte, 1024)
	for {
		n, err := ws.sock.Read(buffer)
		if err != nil {
			return err
		}
		if n == 0 {
			break
		}
	}
	// TODO: consider depleting the send buffer on Linux with ioctl(fd, SIOCOUTQ, &outstanding)
	return ws.sock.Close()
}

func (ws *WS) readEntireBufferRaw(buffer []byte) error {
	for len(buffer) > 0 {
		n, err := ws.sock.Read(buffer)
		if err != nil {
			return err
		}
		buffer = buffer[n:]
	}
	return nil
}

func (ws *WS) writeEntireBufferRaw(buffer []byte) error {
	for len(buffer) > 0 {
		n, err := ws.sock.Write(buffer)
		if err != nil {
			return err
		}
		buffer = buffer[n:]
	}
	return nil
}

func (ws *WS) peekRaw(buffer []byte) (int, error) {
	n, _, err := ws.sock.Recvfrom(context.TODO(), buffer, syscall.MSG_PEEK)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// TODO: make nonblocking version of c3ws::accept

func Accept(sock *socket.Conn) (WS, error) {
	ws := WS{
		sock:   sock,
		Client: false,
	}
	err := ws.serverHandshake()
	if err != nil {
		return WS{}, err
	}
	return ws, nil
}

// TODO: connect should just accept a ws/wss URL

func Connect(sock *socket.Conn, host string, endpoint string) (WS, error) {
	ws := WS{
		sock:   sock,
		Client: true,
	}
	err := ws.clientHandshake(host, endpoint)
	if err != nil {
		return WS{}, err
	}
	return ws, nil
}

func (ws *WS) serverHandshake() error {
	// TODO: Ws.server_handshake assumes that request fits into 1024 bytes
	buffer := make([]byte, 1024)
	bufferSize, err := ws.peekRaw(buffer)
	if err != nil {
		return err
	}
	request := string(buffer[:bufferSize])
	secWebSocketKey, err := parseSecWebSocketKeyFromRequest(&request)
	if err != nil {
		return err
	}
	_, err = ws.sock.Read(buffer[0 : bufferSize-len(request)])
	if err != nil {
		return err
	}
	var handshake strings.Builder
	handshake.Grow(1024)
	handshake.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	handshake.WriteString("Upgrade: websocket\r\n")
	handshake.WriteString("Connection: Upgrade\r\n")
	handshake.WriteString(fmt.Sprintf("Sec-WebSocket-Accept: %s\r\n", computeSecWebSocketAccept(secWebSocketKey)))
	handshake.WriteString("\r\n")
	_, err = ws.sock.Write([]byte(handshake.String()))
	if err != nil {
		return err
	}
	return nil
}

// https://datatracker.ietf.org/doc/html/rfc6455#section-1.3
// TODO: Ws.client_handshake should just accept a ws/wss URL

func (ws *WS) clientHandshake(host, endpoint string) error {
	var handshake strings.Builder
	handshake.Grow(1024)
	// TODO: customizable resource path
	handshake.WriteString(fmt.Sprintf("GET %s HTTP/1.1\r\n", endpoint))
	handshake.WriteString(fmt.Sprintf("Host: %s\r\n", host))
	handshake.WriteString("Upgrade: websocket\r\n")
	handshake.WriteString("Connection: Upgrade\r\n")
	// TODO: Custom WebSocket key
	// Maybe even hardcode something that identifies c3ws?
	handshake.WriteString("Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n")
	handshake.WriteString("Sec-WebSocket-Version: 13\r\n")
	handshake.WriteString("\r\n")
	_, err := ws.sock.Write([]byte(handshake.String()))
	if err != nil {
		return err
	}
	// TODO: Ws.client_handshake assumes that response fits into 1024 bytes
	buffer := make([]byte, 1024)
	bufferSize, err := ws.peekRaw(buffer)
	if err != nil {
		return err
	}
	response := string(buffer[0:bufferSize])
	secWebSocketAccept, err := parseSecWebSocketAcceptFromResponse(&response)
	if err != nil {
		return err
	}
	_, err = ws.sock.Read(buffer[0 : bufferSize-len(response)])
	if err != nil {
		return err
	}
	if secWebSocketAccept != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		return dummyErr
	}
	return nil
}

func (ws *WS) SendFrame(fin bool, opcode WSOpcode, payload []byte) error {
	if ws.Debug {
		fmt.Printf("WSODING DEBUG: TX FRAME: FIN(%v), OPCODE(%s), RSV(000), PAYLOAD_LEN: %d\n", fin, opcode.name(), len(payload))
	}
	// Send FIN and OPCODE
	{
		// NOTE: FIN is always set
		data := byte(opcode)
		if fin {
			data |= (1 << 7)
		}
		ws.writeEntireBufferRaw([]byte{data})
	}
	// Send masked and payload length
	{
		// TODO: do we need to reverse the bytes on a machine with a different endianess than x86?
		// NOTE: client frames are always masked
		if len(payload) < 126 {
			var data byte
			if ws.Client {
				data = 1 << 7
			}
			data |= byte(len(payload))
			err := ws.writeEntireBufferRaw([]byte{data})
			if err != nil {
				return err
			}
		} else if len(payload) <= math.MaxUint16 {
			var data byte
			if ws.Client {
				data = 1 << 7
			}
			data |= 126
			err := ws.writeEntireBufferRaw([]byte{data})
			if err != nil {
				return err
			}
			length := []byte{
				byte((len(payload) >> (8 * 1)) & 0xFF),
				byte((len(payload) >> (8 * 0)) & 0xFF)}
			err = ws.writeEntireBufferRaw(length)
			if err != nil {
				return err
			}

		} else if len(payload) > math.MaxUint16 {
			var data byte
			if ws.Client {
				data = 1 << 7
			}
			data |= 127
			length := []byte{
				byte((len(payload) >> (8 * 7)) & 0xFF),
				byte((len(payload) >> (8 * 6)) & 0xFF),
				byte((len(payload) >> (8 * 5)) & 0xFF),
				byte((len(payload) >> (8 * 4)) & 0xFF),
				byte((len(payload) >> (8 * 3)) & 0xFF),
				byte((len(payload) >> (8 * 2)) & 0xFF),
				byte((len(payload) >> (8 * 1)) & 0xFF),
				byte((len(payload) >> (8 * 0)) & 0xFF)}
			err := ws.writeEntireBufferRaw([]byte{data})
			if err != nil {
				return err
			}
			err = ws.writeEntireBufferRaw(length)
			if err != nil {
				return err
			}
		}
	}
	if ws.Client {
		// Generate and send mask
		mask := make([]byte, 4)
		rand.Read(mask)
		ws.writeEntireBufferRaw(mask)
		for i := 0; i < len(payload); {
			chunk := make([]byte, 1024)
			chunkSize := 0
			for i < len(payload) && chunkSize < len(chunk) {
				chunk[chunkSize] = payload[i] ^ mask[i%4]
				chunkSize++
				i++
			}
			ws.writeEntireBufferRaw(chunk[0:chunkSize])
		}

	} else {
		ws.writeEntireBufferRaw(payload)
	}
	return nil
}

func (ws *WS) SendMessage(kind WSMessageKind, payload []byte) error {
	first := true
	for {
		length := len(payload)
		if length > chunkSize {
			length = chunkSize
		}
		fin := len(payload)-length == 0
		opcode := opCodeCONT
		if first {
			opcode = WSOpcode(kind)
		}
		err := ws.SendFrame(fin, opcode, payload[0:length])
		if err != nil {
			return err
		}
		payload = payload[length:]
		first = false

		if len(payload) == 0 {
			break
		}
	}
	return nil
}

func (ws *WS) sendText(text string) error {
	return ws.SendMessage(messageTEXT, []byte(text))
}

func (ws *WS) sendBinary(binary []byte) error {
	return ws.SendMessage(messageBIN, binary)
}

func (ws *WS) readFrameHeader() (WSFrameHeader, error) {
	header := make([]byte, 2)
	// Read the header
	err := ws.readEntireBufferRaw(header)
	if err != nil {
		return WSFrameHeader{}, err
	}

	frameHeader := WSFrameHeader{
		fin:    Itob(headerMacro(header, "fin")),
		rsv1:   Itob(headerMacro(header, "rsv1")),
		rsv2:   Itob(headerMacro(header, "rsv2")),
		rsv3:   Itob(headerMacro(header, "rsv3")),
		opcode: WSOpcode(headerMacro(header, "opcode")),
		masked: Itob(headerMacro(header, "mask")),
	}
	// Parse the payload length
	{
		// TODO: do we need to reverse the bytes on a machine with a different endianess than x86?
		length := headerMacro(header, "payload_len")
		switch length {
		case 126:
			extLen := make([]byte, 2)
			err := ws.readEntireBufferRaw(extLen)
			if err != nil {
				return WSFrameHeader{}, dummyErr
			}
			for i := 0; i < len(extLen); i++ {
				frameHeader.payloadLen = (frameHeader.payloadLen << 8) | int(extLen[i])
			}
		case 127:
			extLen := make([]byte, 8)
			err := ws.readEntireBufferRaw(extLen)
			if err != nil {
				return WSFrameHeader{}, dummyErr
			}
			for i := 0; i < len(extLen); i++ {
				frameHeader.payloadLen = (frameHeader.payloadLen << 8) | int(extLen[i])
			}
		default:
			frameHeader.payloadLen = int(length)
		}
	}
	if ws.Debug {
		fmt.Printf("WSODING DEBUG: RX FRAME: FIN(%v), OPCODE(%s), RSV(%d%d%d), PAYLOAD_LEN: %d\n", frameHeader.fin, frameHeader.opcode.name(), Btoi(frameHeader.rsv1), Btoi(frameHeader.rsv2), Btoi(frameHeader.rsv3),
			frameHeader.payloadLen)
	}
	// RFC 6455 - Section 5.5:
	// > All control frames MUST have a payload length of 125 bytes or less
	// > and MUST NOT be fragmented.
	if frameHeader.opcode.isControl() && frameHeader.payloadLen > 125 || !frameHeader.fin {
		return WSFrameHeader{}, dummyErr
	}

	// RFC 6455 - Section 5.2:
	// >  RSV1, RSV2, RSV3:  1 bit each
	// >
	// >     MUST be 0 unless an extension is negotiated that defines meanings
	// >     for non-zero values.  If a nonzero value is received and none of
	// >     the negotiated extensions defines the meaning of such a nonzero
	// >     value, the receiving endpoint MUST _Fail the WebSocket
	// >     Connection_.
	if frameHeader.rsv1 || frameHeader.rsv2 || frameHeader.rsv3 {
		return WSFrameHeader{}, dummyErr
	}

	// Read the mask if masked
	if frameHeader.masked {
		err := ws.readEntireBufferRaw(frameHeader.mask[:])
		if err != nil {
			return WSFrameHeader{}, err
		}
	}
	return frameHeader, nil
}

func (ws *WS) readFramePayloadChunk(frameHeader WSFrameHeader, payload []byte, payloadSize int) (int, error) {
	if payloadSize >= len(payload) {
		return 0, nil
	}
	unfinishedPayload := payload[payloadSize:]
	n, err := ws.sock.Read(unfinishedPayload)
	if err != nil {
		return 0, err
	}

	if frameHeader.masked {
		for i := range unfinishedPayload {

			unfinishedPayload[i] ^= frameHeader.mask[(payloadSize+i)%4]
		}
	}
	return n, nil
}

func (ws *WS) readFrameEntirePayload(frameHeader WSFrameHeader) ([]byte, error) {
	payload := make([]byte, frameHeader.payloadLen)
	payloadSize := 0
	for payloadSize < len(payload) {
		n, err := ws.readFramePayloadChunk(frameHeader, payload, payloadSize)
		if err != nil {
			return nil, err
		}
		payloadSize += n
	}
	return payload, nil
}

func (ws *WS) ReadMessage() (*WSMessage, error) {
	var message WSMessage
	payload := make([]byte, 1024)
	var cont bool
	var verifyPos int
loop:
	for {

		frame, err := ws.readFrameHeader()
		if err != nil {
			return nil, err
		}
		if frame.opcode.isControl() {
			switch frame.opcode {
			case opCodeCLOSE:
				return nil, dummyErr
			case opCodePING:
				payload, err := ws.readFrameEntirePayload(frame)
				if err != nil {
					return nil, err
				}
				ws.SendFrame(true, opCodePONG, payload)
			case opCodePONG:
				_, err := ws.readFrameEntirePayload(frame)
				if err != nil {
					return nil, err
				}
				// Unsolicited PONGs are just ignored
				break loop
			default:
				return nil, dummyErr
			}
		} else {
			if !cont {
				switch frame.opcode {
				case opCodeTEXT:
					fallthrough
				case opCodeBIN:
					message.Kind = WSMessageKind(frame.opcode)
				default:
					return nil, dummyErr
				}
				cont = true
			} else {
				if frame.opcode != opCodeCONT {
					return nil, dummyErr
				}
			}
			framePayload := make([]byte, frame.payloadLen)
			var framePayloadSize int
			for framePayloadSize < len(framePayload) {
				n, err := ws.readFramePayloadChunk(frame, framePayload, framePayloadSize)
				if err != nil {
					return nil, err
				}
				payload = append(payload, framePayload[framePayloadSize:n]...)
				framePayloadSize += n
				if message.Kind == messageTEXT {
					// Verifying UTF-8
					for verifyPos < len(payload) {
						size := len(payload) - verifyPos
						if _, err := utf8ToChar32Fixed(unsafe.Pointer(unsafe.SliceData(payload[verifyPos:])), &size); err != nil {
							if errors.Is(err, extraDummy) {
								if !frame.fin {
									savedLen := len(payload)
									extendUnfinishedUtf8(&payload, verifyPos)
									size = len(payload) - verifyPos
									_, err := utf8ToChar32Fixed(unsafe.Pointer(unsafe.SliceData(payload[verifyPos:])), &size)
									if err != nil {
										return nil, err
									}
									payload = payload[:savedLen]
									break // Tolerating the unfinished UTF-8 sequences if the message is unfinished
								}
								return nil, err
							} else {
								return nil, err
							}
						}
						verifyPos += size
					}

				}
			}
		}
		if frame.fin {
			break
		}
	}
	message.Payload = payload
	return &message, nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////

func headerMacro(header []byte, field string) byte {
	switch field {
	case "fin":
		return (header[0] >> 7) & 0x1
	case "rsv1":
		return (header[0] >> 6) & 0x1
	case "rsv2":
		return (header[0] >> 5) & 0x1
	case "rsv3":
		return (header[0] >> 4) & 0x1
	case "opcode":
		return header[0] & 0xF
	case "mask":
		return header[1] >> 7
	case "payload_len":
		return header[1] & 0x7F
	default:
		panic("unreachable")
	}
}

func computeSecWebSocketAccept(secWebSocketKey string) string {
	h := sha1.New()
	io.WriteString(h, secWebSocketKey+"258EAFA5-E914-47DA-95CA-C5AB0DC85B11")
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func parseSecWebSocketKeyFromRequest(request *string) (string, error) {
	foundSecWebSocketKey := false
	var secWebSocketKey string
	const lineSep = "\r\n"
	const headerSep = ":"

	// TODO: verify the request status line
	if index := strings.Index(*request, lineSep); index != -1 {
		*request = (*request)[index+len(lineSep):]
	} else {
		return "", dummyErr
	}
	// TODO: verify the rest of the headers of the request
	// Right now we are only looking for Sec-WebSocket-Key
	for len(*request) > 0 && !strings.HasPrefix(*request, lineSep) {
		var header string
		if index := strings.Index(*request, lineSep); index != -1 {
			header = (*request)[0:index]
			*request = (*request)[index+len(lineSep):]
		} else {
			return "", dummyErr
		}
		var key, value string
		if index := strings.Index(header, headerSep); index != -1 {
			key = strings.TrimSpace(header[0:index])
			value = strings.TrimSpace(header[index+len(headerSep):])
		} else {
			return "", dummyErr
		}
		if key == "Sec-WebSocket-Key" {
			if foundSecWebSocketKey {
				return "", dummyErr
			}
			secWebSocketKey = value
			foundSecWebSocketKey = true
		}

	}
	if !strings.HasPrefix(*request, lineSep) {
		return "", dummyErr
	}
	*request = (*request)[len(lineSep):]
	if !foundSecWebSocketKey {
		return "", dummyErr
	}
	return secWebSocketKey, nil
}

func parseSecWebSocketAcceptFromResponse(response *string) (string, error) {
	foundSecWebSocketAccept := false
	var secWebSocketAccept string
	const lineSep = "\r\n"
	const headerSep = ":"

	// TODO: verify the response status line
	//   If the status code is an error one, log the message
	if index := strings.Index(*response, lineSep); index != -1 {
		*response = (*response)[index+len(lineSep):]
	} else {
		return "", dummyErr
	}
	// TODO: verify the rest of the headers of the response
	// Right now we are only looking for Sec-WebSocket-Accept
	for len(*response) > 0 && !strings.HasPrefix(*response, lineSep) {
		var header string
		if index := strings.Index(*response, lineSep); index != -1 {
			header = (*response)[0:index]
			*response = (*response)[index+len(lineSep):]
		} else {
			return "", dummyErr
		}
		var key, value string
		if index := strings.Index(header, headerSep); index != -1 {
			key = strings.TrimSpace(header[0:index])
			value = strings.TrimSpace(header[index+len(headerSep):])
		} else {
			return "", dummyErr
		}
		if key == "Sec-WebSocket-Accept" {
			if foundSecWebSocketAccept {
				return "", dummyErr
			}
			secWebSocketAccept = value
			foundSecWebSocketAccept = true
		}

	}
	if !strings.HasPrefix(*response, lineSep) {
		return "", dummyErr
	}
	*response = (*response)[len(lineSep):]
	if !foundSecWebSocketAccept {
		return "", dummyErr
	}
	return secWebSocketAccept, nil
}

type WSMessageKind byte

const messageTEXT WSMessageKind = WSMessageKind(opCodeTEXT)
const messageBIN WSMessageKind = WSMessageKind(opCodeBIN)

type WSMessage struct {
	Kind    WSMessageKind
	Payload []byte
}

type WSOpcode byte

const (
	opCodeCONT  WSOpcode = 0x0
	opCodeTEXT  WSOpcode = 0x1
	opCodeBIN   WSOpcode = 0x2
	opCodeCLOSE WSOpcode = 0x8
	opCodePING  WSOpcode = 0x9
	opCodePONG  WSOpcode = 0xA
)

func (opcode WSOpcode) name() string {
	switch opcode {
	case opCodeCONT:
		return "CONT"
	case opCodeTEXT:
		return "TEXT"
	case opCodeBIN:
		return "BIN"
	case opCodeCLOSE:
		return "CLOSE"
	case opCodePING:
		return "PING"
	case opCodePONG:
		return "PONG"
	default:
		if 0x3 <= opcode && opcode <= 0x7 {
			return fmt.Sprintf("NONCONTROL(0x%X)", opcode&0xF)

		} else if 0xB <= opcode && opcode <= 0xF {
			return fmt.Sprintf("CONTROL(0x%X)", opcode&0xF)
		} else {
			return fmt.Sprintf("INVALID(0x%X)", opcode&0xF)
		}
	}
}

func (opcode WSOpcode) isControl() bool {
	return 0x8 <= opcode && opcode <= 0xF
}

type WSFrameHeader struct {
	fin, rsv1, rsv2, rsv3 bool
	opcode                WSOpcode
	masked                bool
	payloadLen            int
	mask                  [4]byte
}

func Btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func Itob(i uint8) bool {
	return i != 0
}

func extendUnfinishedUtf8(payload *[]byte, pos int) {
	c := (*payload)[pos]
	var size int
	if c&0x80 == 0 {
		size = 1
	} else if c&0xE0 == 0xC0 {
		size = 2
	} else if c&0xF0 == 0xE0 {
		size = 3
	} else {
		size = 4
	}
	for len(*payload)-pos < size {
		*payload = append(*payload, 0b1000_0000)
	}
}

func utf8ToChar32Fixed(ptr unsafe.Pointer, size *int) (rune, error) {
	maxSize := *size
	if maxSize < 1 {
		return 0, extraDummy
	}
	var c byte
	c = *(*byte)(ptr)
	byteSize := unsafe.Sizeof(new(byte))
	ptr = unsafe.Add(ptr, byteSize)

	if c&0x80 == 0 {
		*size = 1
		return rune(c), nil
	}
	if c&0xE0 == 0xC0 {
		if maxSize < 2 {
			return 0, extraDummy
		}
		*size = 2
		uc := rune(c&0x1F) << 6
		c = *(*byte)(ptr)
		// Overlong sequence or invalid second.
		if uc == 0 || c&0xC0 != 0x80 {
			return 0, dummyErr
		}
		uc += rune(c & 0x3F)
		// NEW: maximum overlong sequence
		if uc <= 0b111_1111 {
			return 0, dummyErr
		}
		// NEW: UTF-16 surrogate pairs
		if 0xD800 <= uc && uc <= 0xDFFF {
			return 0, dummyErr
		}
		return uc, nil
	}
	if c&0xF0 == 0xE0 {
		if maxSize < 3 {
			return 0, extraDummy
		}
		*size = 3
		uc := rune(c&0x0F) << 12
		c = *(*byte)(ptr)
		// Overlong sequence or invalid last.
		if c&0xC0 != 0x80 {
			return 0, dummyErr
		}
		uc += rune(c&0x3F) << 6
		c = *(*byte)(unsafe.Add(ptr, byteSize))
		if uc == 0 || c&0xC0 != 0x80 {
			return 0, dummyErr
		}
		uc += rune(c & 0x3F)
		// NEW: maximum overlong sequence
		if uc <= 0b11111_111111 {
			return 0, dummyErr
		}
		// NEW: UTF-16 surrogate pairs
		if 0xD800 <= uc && uc <= 0xDFFF {
			return 0, dummyErr
		}
		return uc, nil
	}
	if maxSize < 4 {
		return 0, extraDummy
	}
	*size = 4
	uc := rune(c&0x07) << 18
	c = *(*byte)(ptr)
	ptr = unsafe.Add(ptr, byteSize)
	if c&0xC0 != 0x80 {
		return 0, dummyErr
	}
	uc += rune(c&0x3F) << 12
	c = *(*byte)(ptr)
	ptr = unsafe.Add(ptr, byteSize)
	if c&0xC0 != 0x80 {
		return 0, dummyErr
	}
	uc += rune(c&0x3F) << 6
	c = *(*byte)(ptr)
	if uc == 0 || c&0xC0 != 0x80 {
		return 0, dummyErr
	}
	uc += rune(c & 0x3F)
	// NEW: UTF-16 surrogate pairs
	if 0xD800 <= uc && uc <= 0xDFFF {
		return 0, dummyErr
	}
	if uc <= 0b1111_111111_111111 {
		return 0, dummyErr
	}
	if uc > 0x10FFFF {
		return 0, dummyErr
	}
	return uc, nil
}
