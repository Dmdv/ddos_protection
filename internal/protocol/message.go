package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrMessageTooLarge indicates the message exceeds MaxMessageSize.
	ErrMessageTooLarge = errors.New("message exceeds maximum size")

	// ErrMessageTooShort indicates the message is shorter than minimum (header only).
	ErrMessageTooShort = errors.New("message too short")

	// ErrInvalidMessageType indicates an unknown message type.
	ErrInvalidMessageType = errors.New("invalid message type")

	// ErrUnexpectedEOF indicates connection closed while reading message.
	ErrUnexpectedEOF = errors.New("unexpected end of message")
)

// Message represents a protocol message with type and payload.
type Message struct {
	Type    MessageType
	Payload []byte
}

// NewMessage creates a new message with the given type and payload.
func NewMessage(msgType MessageType, payload []byte) *Message {
	return &Message{
		Type:    msgType,
		Payload: payload,
	}
}

// NewRequestChallenge creates a REQUEST_CHALLENGE message.
func NewRequestChallenge() *Message {
	return &Message{
		Type:    TypeRequestChallenge,
		Payload: nil,
	}
}

// NewChallenge creates a CHALLENGE message with the challenge bytes.
func NewChallenge(challengeBytes []byte) *Message {
	return &Message{
		Type:    TypeChallenge,
		Payload: challengeBytes,
	}
}

// NewSolution creates a SOLUTION message with the solution bytes.
func NewSolution(solutionBytes []byte) *Message {
	return &Message{
		Type:    TypeSolution,
		Payload: solutionBytes,
	}
}

// NewQuote creates a QUOTE message with the quote text.
func NewQuote(quote string) *Message {
	return &Message{
		Type:    TypeQuote,
		Payload: []byte(quote),
	}
}

// NewError creates an ERROR message with the error code.
func NewError(code ErrorCode) *Message {
	return &Message{
		Type:    TypeError,
		Payload: []byte{byte(code)},
	}
}

// Size returns the total wire size of the message (header + payload).
func (m *Message) Size() int {
	return HeaderSize + len(m.Payload)
}

// Validate checks if the message is valid for transmission.
func (m *Message) Validate() error {
	if !m.Type.IsValid() {
		return fmt.Errorf("%w: %d", ErrInvalidMessageType, m.Type)
	}
	if m.Size() > MaxMessageSize {
		return fmt.Errorf("%w: size %d exceeds %d", ErrMessageTooLarge, m.Size(), MaxMessageSize)
	}
	return nil
}

// Marshal encodes the message to wire format.
// Wire format: [length:2][type:1][payload:N]
func (m *Message) Marshal() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}

	length := uint16(m.Size())
	buf := make([]byte, length)

	// Length (2 bytes, big-endian) - includes header + payload
	binary.BigEndian.PutUint16(buf[0:2], length)

	// Type (1 byte)
	buf[2] = byte(m.Type)

	// Payload (N bytes)
	if len(m.Payload) > 0 {
		copy(buf[3:], m.Payload)
	}

	return buf, nil
}

// WriteMessage writes a message to the writer in wire format.
func WriteMessage(w io.Writer, m *Message) error {
	data, err := m.Marshal()
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	n, err := w.Write(data)
	if err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("short write: wrote %d of %d bytes", n, len(data))
	}

	return nil
}

// ReadMessage reads a message from the reader.
// Returns the parsed message or an error.
func ReadMessage(r io.Reader) (*Message, error) {
	// Read length (2 bytes, big-endian)
	lengthBuf := make([]byte, LengthSize)
	if _, err := io.ReadFull(r, lengthBuf); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrUnexpectedEOF
		}
		return nil, fmt.Errorf("read length: %w", err)
	}

	length := binary.BigEndian.Uint16(lengthBuf)

	// Validate length
	if length < HeaderSize {
		return nil, fmt.Errorf("%w: length %d is less than header size %d", ErrMessageTooShort, length, HeaderSize)
	}
	if length > MaxMessageSize {
		return nil, fmt.Errorf("%w: length %d exceeds maximum %d", ErrMessageTooLarge, length, MaxMessageSize)
	}

	// Read remaining bytes (type + payload)
	remaining := int(length) - LengthSize
	buf := make([]byte, remaining)
	if _, err := io.ReadFull(r, buf); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrUnexpectedEOF
		}
		return nil, fmt.Errorf("read payload: %w", err)
	}

	// Parse type
	msgType := MessageType(buf[0])

	// Extract payload
	var payload []byte
	if len(buf) > 1 {
		payload = buf[1:]
	}

	return &Message{
		Type:    msgType,
		Payload: payload,
	}, nil
}

// UnmarshalMessage parses a message from a complete wire-format buffer.
func UnmarshalMessage(data []byte) (*Message, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("%w: got %d bytes, need at least %d", ErrMessageTooShort, len(data), HeaderSize)
	}

	length := binary.BigEndian.Uint16(data[0:2])

	if int(length) != len(data) {
		return nil, fmt.Errorf("length mismatch: header says %d, got %d bytes", length, len(data))
	}

	if length > MaxMessageSize {
		return nil, fmt.Errorf("%w: length %d exceeds maximum %d", ErrMessageTooLarge, length, MaxMessageSize)
	}

	msgType := MessageType(data[2])

	var payload []byte
	if len(data) > HeaderSize {
		payload = data[HeaderSize:]
	}

	return &Message{
		Type:    msgType,
		Payload: payload,
	}, nil
}

// GetErrorCode extracts the error code from an ERROR message payload.
// Returns ErrCodeInvalidMessage if the message is not an ERROR or has invalid payload.
func (m *Message) GetErrorCode() ErrorCode {
	if m.Type != TypeError || len(m.Payload) < 1 {
		return ErrCodeInvalidMessage
	}
	return ErrorCode(m.Payload[0])
}

// String returns a human-readable representation of the message.
func (m *Message) String() string {
	switch m.Type {
	case TypeError:
		if len(m.Payload) > 0 {
			return fmt.Sprintf("%s: %s", m.Type, ErrorCode(m.Payload[0]))
		}
		return m.Type.String()
	case TypeQuote:
		return fmt.Sprintf("%s: %q", m.Type, string(m.Payload))
	default:
		return fmt.Sprintf("%s (payload: %d bytes)", m.Type, len(m.Payload))
	}
}
