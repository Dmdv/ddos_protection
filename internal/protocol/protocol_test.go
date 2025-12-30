package protocol

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
)

// TestMessageType_String tests message type string representation
func TestMessageType_String(t *testing.T) {
	tests := []struct {
		msgType  MessageType
		expected string
	}{
		{TypeRequestChallenge, "REQUEST_CHALLENGE"},
		{TypeChallenge, "CHALLENGE"},
		{TypeSolution, "SOLUTION"},
		{TypeQuote, "QUOTE"},
		{TypeError, "ERROR"},
		{MessageType(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.msgType.String(); got != tt.expected {
				t.Errorf("MessageType.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestMessageType_IsValid tests message type validation
func TestMessageType_IsValid(t *testing.T) {
	tests := []struct {
		msgType MessageType
		valid   bool
	}{
		{TypeRequestChallenge, true},
		{TypeChallenge, true},
		{TypeSolution, true},
		{TypeQuote, true},
		{TypeError, true},
		{MessageType(0), false},
		{MessageType(6), false},
		{MessageType(255), false},
	}

	for _, tt := range tests {
		if got := tt.msgType.IsValid(); got != tt.valid {
			t.Errorf("MessageType(%d).IsValid() = %v, want %v", tt.msgType, got, tt.valid)
		}
	}
}

// TestErrorCode_String tests error code string representation
func TestErrorCode_String(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected string
	}{
		{ErrCodeInvalidMessage, "ERR_INVALID_MESSAGE"},
		{ErrCodeInvalidState, "ERR_INVALID_STATE"},
		{ErrCodeChallengeExpired, "ERR_CHALLENGE_EXPIRED"},
		{ErrCodeInvalidSignature, "ERR_INVALID_SIGNATURE"},
		{ErrCodeInvalidSolution, "ERR_INVALID_SOLUTION"},
		{ErrCodeTooManyAttempts, "ERR_TOO_MANY_ATTEMPTS"},
		{ErrCodeRateLimited, "ERR_RATE_LIMITED"},
		{ErrCodeServerBusy, "ERR_SERVER_BUSY"},
		{ErrorCode(99), "ERR_UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.code.String(); got != tt.expected {
				t.Errorf("ErrorCode.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestErrorCode_IsValid tests error code validation
func TestErrorCode_IsValid(t *testing.T) {
	tests := []struct {
		code  ErrorCode
		valid bool
	}{
		{ErrCodeInvalidMessage, true},
		{ErrCodeServerBusy, true},
		{ErrorCode(0), false},
		{ErrorCode(9), false},
	}

	for _, tt := range tests {
		if got := tt.code.IsValid(); got != tt.valid {
			t.Errorf("ErrorCode(%d).IsValid() = %v, want %v", tt.code, got, tt.valid)
		}
	}
}

// TestErrorCode_IsRetryable tests retryable error detection
func TestErrorCode_IsRetryable(t *testing.T) {
	tests := []struct {
		code      ErrorCode
		retryable bool
	}{
		{ErrCodeInvalidMessage, false},
		{ErrCodeInvalidState, false},
		{ErrCodeChallengeExpired, false},
		{ErrCodeInvalidSignature, false},
		{ErrCodeInvalidSolution, true},  // Can retry
		{ErrCodeTooManyAttempts, false}, // Already retried 3 times
		{ErrCodeRateLimited, true},      // Wait and retry
		{ErrCodeServerBusy, true},       // Wait and retry
	}

	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			if got := tt.code.IsRetryable(); got != tt.retryable {
				t.Errorf("ErrorCode.IsRetryable() = %v, want %v", got, tt.retryable)
			}
		})
	}
}

// TestErrorCode_Description tests error descriptions
func TestErrorCode_Description(t *testing.T) {
	// Just verify each code has a non-empty description
	codes := []ErrorCode{
		ErrCodeInvalidMessage,
		ErrCodeInvalidState,
		ErrCodeChallengeExpired,
		ErrCodeInvalidSignature,
		ErrCodeInvalidSolution,
		ErrCodeTooManyAttempts,
		ErrCodeRateLimited,
		ErrCodeServerBusy,
	}

	for _, code := range codes {
		desc := code.Description()
		if desc == "" {
			t.Errorf("ErrorCode(%d).Description() returned empty string", code)
		}
	}
}

// TestMessage_Marshal tests message marshaling
func TestMessage_Marshal(t *testing.T) {
	tests := []struct {
		name     string
		msg      *Message
		expected []byte
		wantErr  bool
	}{
		{
			name:     "request challenge",
			msg:      NewRequestChallenge(),
			expected: []byte{0x00, 0x03, 0x01},
			wantErr:  false,
		},
		{
			name:     "error message",
			msg:      NewError(ErrCodeInvalidSolution),
			expected: []byte{0x00, 0x04, 0x05, 0x05},
			wantErr:  false,
		},
		{
			name:     "quote message",
			msg:      NewQuote("Hello"),
			expected: []byte{0x00, 0x08, 0x04, 'H', 'e', 'l', 'l', 'o'},
			wantErr:  false,
		},
		{
			name:    "invalid type",
			msg:     &Message{Type: MessageType(0), Payload: nil},
			wantErr: true,
		},
		{
			name:    "payload too large",
			msg:     &Message{Type: TypeQuote, Payload: make([]byte, MaxPayloadSize+1)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.msg.Marshal()

			if tt.wantErr {
				if err == nil {
					t.Error("Marshal() should return error")
				}
				return
			}

			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			if !bytes.Equal(data, tt.expected) {
				t.Errorf("Marshal() = %x, want %x", data, tt.expected)
			}
		})
	}
}

// TestMessage_RoundTrip tests marshal/unmarshal round-trip
func TestMessage_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  *Message
	}{
		{"request challenge", NewRequestChallenge()},
		{"error", NewError(ErrCodeRateLimited)},
		{"quote short", NewQuote("Test")},
		{"quote long", NewQuote("The only true wisdom is in knowing you know nothing.")},
		{"challenge", NewChallenge(make([]byte, 90))},
		{"solution", NewSolution(make([]byte, 98))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := tt.msg.Marshal()
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			// Unmarshal
			restored, err := UnmarshalMessage(data)
			if err != nil {
				t.Fatalf("UnmarshalMessage() error = %v", err)
			}

			// Compare
			if restored.Type != tt.msg.Type {
				t.Errorf("Type = %v, want %v", restored.Type, tt.msg.Type)
			}
			if !bytes.Equal(restored.Payload, tt.msg.Payload) {
				t.Errorf("Payload mismatch")
			}
		})
	}
}

// TestUnmarshalMessage_Invalid tests invalid message handling
func TestUnmarshalMessage_Invalid(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"too short", []byte{0x00, 0x03}},
		{"length mismatch", []byte{0x00, 0x05, 0x01}}, // Says 5, but only 3 bytes
		{"too large", func() []byte {
			data := make([]byte, MaxMessageSize+1)
			data[0] = 0x04
			data[1] = 0x01
			return data
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := UnmarshalMessage(tt.data)
			if err == nil {
				t.Error("UnmarshalMessage() should return error")
			}
		})
	}
}

// TestWriteMessage tests writing messages
func TestWriteMessage(t *testing.T) {
	msg := NewRequestChallenge()

	var buf bytes.Buffer
	err := WriteMessage(&buf, msg)
	if err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}

	expected := []byte{0x00, 0x03, 0x01}
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("WriteMessage() wrote %x, want %x", buf.Bytes(), expected)
	}
}

// TestReadMessage tests reading messages
func TestReadMessage(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "valid request challenge",
			data:    []byte{0x00, 0x03, 0x01},
			wantErr: false,
		},
		{
			name:    "valid error",
			data:    []byte{0x00, 0x04, 0x05, 0x05},
			wantErr: false,
		},
		{
			name:    "length too short",
			data:    []byte{0x00, 0x02, 0x01}, // Length 2 < HeaderSize
			wantErr: true,
		},
		{
			name: "length too large",
			data: func() []byte {
				data := make([]byte, MaxMessageSize+10)
				data[0] = 0x04
				data[1] = 0x10 // Length > MaxMessageSize
				return data
			}(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader(tt.data)
			msg, err := ReadMessage(reader)

			if tt.wantErr {
				if err == nil {
					t.Error("ReadMessage() should return error")
				}
				return
			}

			if err != nil {
				t.Fatalf("ReadMessage() error = %v", err)
			}
			if msg == nil {
				t.Error("ReadMessage() returned nil message")
			}
		})
	}
}

// TestReadMessage_EOF tests EOF handling
func TestReadMessage_EOF(t *testing.T) {
	// Empty reader
	reader := bytes.NewReader([]byte{})
	_, err := ReadMessage(reader)
	if !errors.Is(err, io.EOF) {
		t.Errorf("ReadMessage() error = %v, want io.EOF", err)
	}
}

// TestReadMessage_UnexpectedEOF tests partial message handling
func TestReadMessage_UnexpectedEOF(t *testing.T) {
	// Partial length field
	reader := bytes.NewReader([]byte{0x00})
	_, err := ReadMessage(reader)
	if err == nil {
		t.Error("ReadMessage() should return error for partial length")
	}

	// Length says 10 bytes, but only 3 available
	reader = bytes.NewReader([]byte{0x00, 0x0A, 0x01})
	_, err = ReadMessage(reader)
	if err == nil {
		t.Error("ReadMessage() should return error for truncated payload")
	}
}

// TestMessage_GetErrorCode tests error code extraction
func TestMessage_GetErrorCode(t *testing.T) {
	tests := []struct {
		name     string
		msg      *Message
		expected ErrorCode
	}{
		{
			name:     "valid error",
			msg:      NewError(ErrCodeRateLimited),
			expected: ErrCodeRateLimited,
		},
		{
			name:     "not an error message",
			msg:      NewRequestChallenge(),
			expected: ErrCodeInvalidMessage,
		},
		{
			name:     "empty error payload",
			msg:      &Message{Type: TypeError, Payload: nil},
			expected: ErrCodeInvalidMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.msg.GetErrorCode(); got != tt.expected {
				t.Errorf("GetErrorCode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestMessage_String tests string representation
func TestMessage_String(t *testing.T) {
	tests := []struct {
		msg      *Message
		contains string
	}{
		{NewRequestChallenge(), "REQUEST_CHALLENGE"},
		{NewError(ErrCodeServerBusy), "ERR_SERVER_BUSY"},
		{NewQuote("Test"), "Test"},
		{NewChallenge(make([]byte, 90)), "90 bytes"},
	}

	for _, tt := range tests {
		s := tt.msg.String()
		if !bytes.Contains([]byte(s), []byte(tt.contains)) {
			t.Errorf("Message.String() = %q, want to contain %q", s, tt.contains)
		}
	}
}

// TestProtocolError tests ProtocolError
func TestProtocolError(t *testing.T) {
	err := NewProtocolError(ErrCodeRateLimited)

	// Test Error()
	errStr := err.Error()
	if errStr == "" {
		t.Error("ProtocolError.Error() returned empty string")
	}

	// Test Is()
	if !errors.Is(err, ErrRateLimited) {
		t.Error("errors.Is() should match ErrRateLimited")
	}
	if errors.Is(err, ErrServerBusy) {
		t.Error("errors.Is() should not match ErrServerBusy")
	}
}

// TestErrorFromCode tests error code to error conversion
func TestErrorFromCode(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected error
	}{
		{ErrCodeInvalidMessage, ErrInvalidMessage},
		{ErrCodeInvalidState, ErrInvalidState},
		{ErrCodeChallengeExpired, ErrChallengeExpired},
		{ErrCodeInvalidSignature, ErrInvalidSignature},
		{ErrCodeInvalidSolution, ErrInvalidSolution},
		{ErrCodeTooManyAttempts, ErrTooManyAttempts},
		{ErrCodeRateLimited, ErrRateLimited},
		{ErrCodeServerBusy, ErrServerBusy},
	}

	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			err := ErrorFromCode(tt.code)
			if !errors.Is(err, tt.expected) {
				t.Errorf("ErrorFromCode() = %v, want %v", err, tt.expected)
			}
		})
	}

	// Test unknown code
	err := ErrorFromCode(ErrorCode(99))
	var pe *ProtocolError
	if !errors.As(err, &pe) {
		t.Error("ErrorFromCode() should return ProtocolError for unknown code")
	}
}

// TestIsRetryableError tests retryable error detection
func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		err       error
		retryable bool
	}{
		{ErrRateLimited, true},
		{ErrServerBusy, true},
		{ErrInvalidSolution, true},
		{ErrInvalidMessage, false},
		{ErrTooManyAttempts, false},
		{errors.New("random error"), false},
	}

	for _, tt := range tests {
		if got := IsRetryableError(tt.err); got != tt.retryable {
			t.Errorf("IsRetryableError(%v) = %v, want %v", tt.err, got, tt.retryable)
		}
	}
}

// TestShouldReconnect tests reconnect logic
func TestShouldReconnect(t *testing.T) {
	tests := []struct {
		err       error
		reconnect bool
	}{
		{ErrInvalidMessage, true},
		{ErrInvalidState, true},
		{ErrChallengeExpired, true},
		{ErrInvalidSignature, true},
		{ErrTooManyAttempts, true},
		{ErrInvalidSolution, false}, // Can retry with current connection
		{ErrRateLimited, false},     // Wait but can reuse connection
		{ErrServerBusy, false},      // Wait but can reuse connection
	}

	for _, tt := range tests {
		var pe *ProtocolError
		if errors.As(tt.err, &pe) {
			t.Run(pe.Code.String(), func(t *testing.T) {
				if got := ShouldReconnect(tt.err); got != tt.reconnect {
					t.Errorf("ShouldReconnect() = %v, want %v", got, tt.reconnect)
				}
			})
		}
	}
}

// TestParseErrorMessage tests error message parsing
func TestParseErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      *Message
		expected error
	}{
		{
			name:     "valid error",
			msg:      NewError(ErrCodeRateLimited),
			expected: ErrRateLimited,
		},
		{
			name:     "not error message",
			msg:      NewRequestChallenge(),
			expected: nil,
		},
		{
			name:     "empty payload",
			msg:      &Message{Type: TypeError, Payload: nil},
			expected: ErrInvalidMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ParseErrorMessage(tt.msg)
			if tt.expected == nil {
				if err != nil {
					t.Errorf("ParseErrorMessage() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.expected) {
				t.Errorf("ParseErrorMessage() = %v, want %v", err, tt.expected)
			}
		})
	}
}

// TestIntegration_NetPipe tests protocol over net.Pipe
func TestIntegration_NetPipe(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Send message from client to server
	go func() {
		msg := NewRequestChallenge()
		if err := WriteMessage(client, msg); err != nil {
			t.Errorf("WriteMessage() error = %v", err)
		}
	}()

	// Read message on server
	msg, err := ReadMessage(server)
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if msg.Type != TypeRequestChallenge {
		t.Errorf("Type = %v, want TypeRequestChallenge", msg.Type)
	}

	// Server sends response
	go func() {
		resp := NewChallenge(make([]byte, 90))
		if err := WriteMessage(server, resp); err != nil {
			t.Errorf("WriteMessage() error = %v", err)
		}
	}()

	// Client reads response
	resp, err := ReadMessage(client)
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if resp.Type != TypeChallenge {
		t.Errorf("Type = %v, want TypeChallenge", resp.Type)
	}
	if len(resp.Payload) != 90 {
		t.Errorf("Payload length = %d, want 90", len(resp.Payload))
	}
}

// TestIntegration_FullFlow tests complete protocol flow over net.Pipe
func TestIntegration_FullFlow(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Simulate server
	serverDone := make(chan error, 1)
	go func() {
		// 1. Read REQUEST_CHALLENGE
		msg, err := ReadMessage(server)
		if err != nil {
			serverDone <- err
			return
		}
		if msg.Type != TypeRequestChallenge {
			serverDone <- errors.New("expected REQUEST_CHALLENGE")
			return
		}

		// 2. Send CHALLENGE
		if err := WriteMessage(server, NewChallenge(make([]byte, 90))); err != nil {
			serverDone <- err
			return
		}

		// 3. Read SOLUTION
		msg, err = ReadMessage(server)
		if err != nil {
			serverDone <- err
			return
		}
		if msg.Type != TypeSolution {
			serverDone <- errors.New("expected SOLUTION")
			return
		}

		// 4. Send QUOTE
		if err := WriteMessage(server, NewQuote("The only true wisdom...")); err != nil {
			serverDone <- err
			return
		}

		serverDone <- nil
	}()

	// Simulate client
	// 1. Send REQUEST_CHALLENGE
	if err := WriteMessage(client, NewRequestChallenge()); err != nil {
		t.Fatalf("Client: send REQUEST_CHALLENGE error = %v", err)
	}

	// 2. Read CHALLENGE
	msg, err := ReadMessage(client)
	if err != nil {
		t.Fatalf("Client: read CHALLENGE error = %v", err)
	}
	if msg.Type != TypeChallenge {
		t.Fatalf("Client: expected CHALLENGE, got %v", msg.Type)
	}

	// 3. Send SOLUTION
	if err := WriteMessage(client, NewSolution(make([]byte, 98))); err != nil {
		t.Fatalf("Client: send SOLUTION error = %v", err)
	}

	// 4. Read QUOTE
	msg, err = ReadMessage(client)
	if err != nil {
		t.Fatalf("Client: read QUOTE error = %v", err)
	}
	if msg.Type != TypeQuote {
		t.Fatalf("Client: expected QUOTE, got %v", msg.Type)
	}

	// Wait for server
	if err := <-serverDone; err != nil {
		t.Fatalf("Server error: %v", err)
	}

	t.Logf("Quote received: %s", string(msg.Payload))
}

// TestIntegration_ErrorFlow tests error handling over net.Pipe
func TestIntegration_ErrorFlow(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Server sends error
	go func() {
		msg, _ := ReadMessage(server)
		if msg.Type == TypeRequestChallenge {
			WriteMessage(server, NewError(ErrCodeRateLimited))
		}
	}()

	// Client flow
	WriteMessage(client, NewRequestChallenge())

	msg, err := ReadMessage(client)
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}

	if msg.Type != TypeError {
		t.Fatalf("Expected ERROR, got %v", msg.Type)
	}

	protocolErr := ParseErrorMessage(msg)
	if !errors.Is(protocolErr, ErrRateLimited) {
		t.Errorf("Expected ErrRateLimited, got %v", protocolErr)
	}

	if !IsRetryableError(protocolErr) {
		t.Error("ErrRateLimited should be retryable")
	}
}

// BenchmarkMessage_Marshal benchmarks message marshaling
func BenchmarkMessage_Marshal(b *testing.B) {
	msg := NewChallenge(make([]byte, 90))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		msg.Marshal()
	}
}

// BenchmarkMessage_WriteRead benchmarks write/read cycle
func BenchmarkMessage_WriteRead(b *testing.B) {
	msg := NewChallenge(make([]byte, 90))
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		WriteMessage(&buf, msg)
		ReadMessage(&buf)
	}
}
