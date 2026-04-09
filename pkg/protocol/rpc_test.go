package protocol

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestNewRequest(t *testing.T) {
	tests := []struct {
		name    string
		id      int
		method  string
		params  any
		wantErr bool
	}{
		{
			name:    "simple request without params",
			id:      1,
			method:  "system.ping",
			params:  nil,
			wantErr: false,
		},
		{
			name:   "request with struct params",
			id:     42,
			method: "killswitch.enable",
			params: struct {
				Interface string `json:"interface"`
			}{Interface: "wg0"},
			wantErr: false,
		},
		{
			name:    "request with map params",
			id:      100,
			method:  "dns.configure",
			params:  map[string]string{"server": "1.1.1.1"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := NewRequest(tt.id, tt.method, tt.params)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if req.JSONRPC != JSONRPCVersion {
				t.Errorf("JSONRPC = %q, want %q", req.JSONRPC, JSONRPCVersion)
			}

			if req.ID != tt.id {
				t.Errorf("ID = %d, want %d", req.ID, tt.id)
			}

			if req.Method != tt.method {
				t.Errorf("Method = %q, want %q", req.Method, tt.method)
			}

			if tt.params == nil && req.Params != nil {
				t.Errorf("Params should be nil, got %s", req.Params)
			}

			if tt.params != nil && req.Params == nil {
				t.Errorf("Params should not be nil")
			}
		})
	}
}

func TestNewResponse(t *testing.T) {
	tests := []struct {
		name    string
		id      int
		result  any
		wantErr bool
	}{
		{
			name:    "response without result",
			id:      1,
			result:  nil,
			wantErr: false,
		},
		{
			name:    "response with string result",
			id:      2,
			result:  "pong",
			wantErr: false,
		},
		{
			name: "response with struct result",
			id:   3,
			result: struct {
				Enabled bool `json:"enabled"`
			}{Enabled: true},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := NewResponse(tt.id, tt.result)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewResponse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			if resp.JSONRPC != JSONRPCVersion {
				t.Errorf("JSONRPC = %q, want %q", resp.JSONRPC, JSONRPCVersion)
			}

			if resp.ID != tt.id {
				t.Errorf("ID = %d, want %d", resp.ID, tt.id)
			}

			if resp.Error != nil {
				t.Errorf("Error should be nil, got %v", resp.Error)
			}

			if !resp.IsSuccess() {
				t.Errorf("IsSuccess() should return true")
			}
		})
	}
}

func TestNewErrorResponse(t *testing.T) {
	tests := []struct {
		name    string
		id      int
		code    int
		message string
		data    any
		wantErr string
	}{
		{
			name:    "simple error",
			id:      1,
			code:    ErrCodeMethodNotFound,
			message: "Method not found",
			data:    nil,
			wantErr: "RPC error -32601: Method not found",
		},
		{
			name:    "error with data",
			id:      2,
			code:    ErrCodeInvalidParams,
			message: "Invalid params",
			data:    "missing field: interface",
			wantErr: "RPC error -32602: Invalid params (data: missing field: interface)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := NewErrorResponse(tt.id, tt.code, tt.message, tt.data)

			if resp.JSONRPC != JSONRPCVersion {
				t.Errorf("JSONRPC = %q, want %q", resp.JSONRPC, JSONRPCVersion)
			}

			if resp.ID != tt.id {
				t.Errorf("ID = %d, want %d", resp.ID, tt.id)
			}

			if resp.Error == nil {
				t.Fatal("Error should not be nil")
			}

			if resp.Error.Code != tt.code {
				t.Errorf("Error.Code = %d, want %d", resp.Error.Code, tt.code)
			}

			if resp.Error.Message != tt.message {
				t.Errorf("Error.Message = %q, want %q", resp.Error.Message, tt.message)
			}

			if resp.IsSuccess() {
				t.Errorf("IsSuccess() should return false for error response")
			}

			if got := resp.Error.Error(); got != tt.wantErr {
				t.Errorf("Error.Error() = %q, want %q", got, tt.wantErr)
			}
		})
	}
}

func TestRequestUnmarshalParams(t *testing.T) {
	type TestParams struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	tests := []struct {
		name    string
		params  json.RawMessage
		want    TestParams
		wantErr bool
	}{
		{
			name:    "valid params",
			params:  json.RawMessage(`{"name":"test","value":42}`),
			want:    TestParams{Name: "test", Value: 42},
			wantErr: false,
		},
		{
			name:    "nil params",
			params:  nil,
			want:    TestParams{},
			wantErr: false,
		},
		{
			name:    "invalid json",
			params:  json.RawMessage(`{invalid}`),
			want:    TestParams{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{Params: tt.params}
			var got TestParams

			err := req.UnmarshalParams(&got)

			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("UnmarshalParams() got = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestResponseUnmarshalResult(t *testing.T) {
	type TestResult struct {
		Enabled bool   `json:"enabled"`
		Status  string `json:"status"`
	}

	tests := []struct {
		name    string
		result  json.RawMessage
		want    TestResult
		wantErr bool
	}{
		{
			name:    "valid result",
			result:  json.RawMessage(`{"enabled":true,"status":"active"}`),
			want:    TestResult{Enabled: true, Status: "active"},
			wantErr: false,
		},
		{
			name:    "nil result",
			result:  nil,
			want:    TestResult{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{Result: tt.result}
			var got TestResult

			err := resp.UnmarshalResult(&got)

			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalResult() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("UnmarshalResult() got = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestErrorHelpers(t *testing.T) {
	tests := []struct {
		name     string
		fn       func() *Response
		wantCode int
	}{
		{"ParseError", func() *Response { return ParseError(1) }, ErrCodeParse},
		{"InvalidRequestError", func() *Response { return InvalidRequestError(1) }, ErrCodeInvalidRequest},
		{"MethodNotFoundError", func() *Response { return MethodNotFoundError(1, "foo") }, ErrCodeMethodNotFound},
		{"InvalidParamsError", func() *Response { return InvalidParamsError(1, "bad") }, ErrCodeInvalidParams},
		{"InternalError", func() *Response { return InternalError(1, io.EOF) }, ErrCodeInternal},
		{"UnauthorizedError", func() *Response { return UnauthorizedError(1) }, ErrCodeUnauthorized},
		{"OperationFailedError", func() *Response { return OperationFailedError(1, io.EOF) }, ErrCodeOperationFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := tt.fn()

			if resp.Error == nil {
				t.Fatal("Error should not be nil")
			}

			if resp.Error.Code != tt.wantCode {
				t.Errorf("Error.Code = %d, want %d", resp.Error.Code, tt.wantCode)
			}
		})
	}
}

// mockConn implements io.ReadWriteCloser for testing
type mockConn struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	closed   bool
}

func newMockConn() *mockConn {
	return &mockConn{
		readBuf:  new(bytes.Buffer),
		writeBuf: new(bytes.Buffer),
	}
}

func (m *mockConn) Read(p []byte) (n int, err error) {
	if m.closed {
		return 0, io.EOF
	}
	return m.readBuf.Read(p)
}

func (m *mockConn) Write(p []byte) (n int, err error) {
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	return m.writeBuf.Write(p)
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func TestCodecWriteRequest(t *testing.T) {
	conn := newMockConn()
	codec := NewCodec(conn)

	req, err := NewRequest(1, "test.method", map[string]int{"value": 42})
	if err != nil {
		t.Fatalf("NewRequest failed: %v", err)
	}

	if err := codec.WriteRequest(req); err != nil {
		t.Fatalf("WriteRequest failed: %v", err)
	}

	// Verify output ends with newline
	output := conn.writeBuf.String()
	if !strings.HasSuffix(output, "\n") {
		t.Errorf("Output should end with newline, got: %q", output)
	}

	// Verify it's valid JSON
	var parsed Request
	if err := json.Unmarshal([]byte(strings.TrimSuffix(output, "\n")), &parsed); err != nil {
		t.Errorf("Output is not valid JSON: %v", err)
	}

	if parsed.Method != "test.method" {
		t.Errorf("Method = %q, want %q", parsed.Method, "test.method")
	}
}

func TestCodecReadRequest(t *testing.T) {
	conn := newMockConn()
	conn.readBuf.WriteString(`{"jsonrpc":"2.0","method":"test.ping","id":123}` + "\n")

	codec := NewCodec(conn)

	req, err := codec.ReadRequest()
	if err != nil {
		t.Fatalf("ReadRequest failed: %v", err)
	}

	if req.Method != "test.ping" {
		t.Errorf("Method = %q, want %q", req.Method, "test.ping")
	}

	if req.ID != 123 {
		t.Errorf("ID = %d, want %d", req.ID, 123)
	}
}

func TestCodecRoundtrip(t *testing.T) {
	// Create a pipe for bidirectional communication
	serverConn := newMockConn()
	clientConn := newMockConn()

	serverCodec := NewCodec(serverConn)
	clientCodec := NewCodec(clientConn)

	// Simulate client sending request
	req, _ := NewRequest(1, "system.ping", nil)
	clientConn.writeBuf.Reset()
	if err := clientCodec.WriteRequest(req); err != nil {
		t.Fatalf("WriteRequest failed: %v", err)
	}

	// Copy client output to server input
	serverConn.readBuf.Write(clientConn.writeBuf.Bytes())

	// Server reads request
	receivedReq, err := serverCodec.ReadRequest()
	if err != nil {
		t.Fatalf("Server ReadRequest failed: %v", err)
	}

	if receivedReq.Method != "system.ping" {
		t.Errorf("Received Method = %q, want %q", receivedReq.Method, "system.ping")
	}

	// Server sends response
	resp, _ := NewResponse(receivedReq.ID, "pong")
	serverConn.writeBuf.Reset()
	if err := serverCodec.WriteResponse(resp); err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	// Copy server output to client input
	clientConn.readBuf.Write(serverConn.writeBuf.Bytes())

	// Client reads response
	receivedResp, err := clientCodec.ReadResponse()
	if err != nil {
		t.Fatalf("Client ReadResponse failed: %v", err)
	}

	if receivedResp.ID != 1 {
		t.Errorf("Response ID = %d, want %d", receivedResp.ID, 1)
	}

	var result string
	if err := receivedResp.UnmarshalResult(&result); err != nil {
		t.Fatalf("UnmarshalResult failed: %v", err)
	}

	if result != "pong" {
		t.Errorf("Result = %q, want %q", result, "pong")
	}
}

func TestCodecClose(t *testing.T) {
	conn := newMockConn()
	codec := NewCodec(conn)

	if codec.IsClosed() {
		t.Error("Codec should not be closed initially")
	}

	if err := codec.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if !codec.IsClosed() {
		t.Error("Codec should be closed after Close()")
	}

	// Operations should fail after close
	req, _ := NewRequest(1, "test", nil)
	if err := codec.WriteRequest(req); err != ErrConnectionClosed {
		t.Errorf("WriteRequest after close should return ErrConnectionClosed, got: %v", err)
	}
}

func TestEncoderDecoder(t *testing.T) {
	var buf bytes.Buffer

	enc := NewEncoder(&buf)
	dec := NewDecoder(&buf)

	// Encode a request
	req, _ := NewRequest(1, "test.method", nil)
	if err := enc.Encode(req); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Decode it back
	decoded, err := dec.DecodeRequest()
	if err != nil {
		t.Fatalf("DecodeRequest failed: %v", err)
	}

	if decoded.Method != req.Method {
		t.Errorf("Method = %q, want %q", decoded.Method, req.Method)
	}
}
