// Package protocol provides JSON-RPC 2.0 message types and client implementation
// for communication between VPN Manager GUI/CLI and the privileged daemon.
//
// The protocol uses newline-delimited JSON over Unix sockets for IPC.
// Authentication is handled via SO_PEERCRED for peer identity verification.
package protocol

import (
	"encoding/json"
	"fmt"
)

// JSONRPCVersion is the JSON-RPC protocol version we implement.
const JSONRPCVersion = "2.0"

// Request represents a JSON-RPC 2.0 request message.
// See: https://www.jsonrpc.org/specification#request_object
type Request struct {
	// JSONRPC specifies the protocol version. MUST be "2.0".
	JSONRPC string `json:"jsonrpc"`

	// Method is the name of the method to invoke.
	// Format: "category.action" (e.g., "killswitch.enable", "dns.status")
	Method string `json:"method"`

	// Params holds the method parameters. Can be nil for methods without params.
	Params json.RawMessage `json:"params,omitempty"`

	// ID uniquely identifies this request. Used to match responses.
	ID int `json:"id"`
}

// Response represents a JSON-RPC 2.0 response message.
// See: https://www.jsonrpc.org/specification#response_object
type Response struct {
	// JSONRPC specifies the protocol version. MUST be "2.0".
	JSONRPC string `json:"jsonrpc"`

	// Result contains the method result on success. Nil on error.
	Result json.RawMessage `json:"result,omitempty"`

	// Error contains error details on failure. Nil on success.
	Error *RPCError `json:"error,omitempty"`

	// ID matches the request ID. Null for notifications.
	ID int `json:"id"`
}

// RPCError represents a JSON-RPC 2.0 error object.
// See: https://www.jsonrpc.org/specification#error_object
type RPCError struct {
	// Code is a numeric error code.
	Code int `json:"code"`

	// Message is a short description of the error.
	Message string `json:"message"`

	// Data contains additional error information. Optional.
	Data any `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *RPCError) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("RPC error %d: %s (data: %v)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// Standard JSON-RPC 2.0 error codes.
// See: https://www.jsonrpc.org/specification#error_object
const (
	// ErrCodeParse indicates invalid JSON was received.
	ErrCodeParse = -32700

	// ErrCodeInvalidRequest indicates the JSON is not a valid Request object.
	ErrCodeInvalidRequest = -32600

	// ErrCodeMethodNotFound indicates the method does not exist.
	ErrCodeMethodNotFound = -32601

	// ErrCodeInvalidParams indicates invalid method parameters.
	ErrCodeInvalidParams = -32602

	// ErrCodeInternal indicates an internal JSON-RPC error.
	ErrCodeInternal = -32603
)

// Application-specific error codes (must be in range -32000 to -32099).
const (
	// ErrCodeUnauthorized indicates the client is not authorized for this operation.
	ErrCodeUnauthorized = -32001

	// ErrCodeOperationFailed indicates the privileged operation failed.
	ErrCodeOperationFailed = -32002

	// ErrCodeTimeout indicates the operation timed out.
	ErrCodeTimeout = -32003

	// ErrCodeNotAvailable indicates a required resource is not available.
	ErrCodeNotAvailable = -32004
)

// NewRequest creates a new JSON-RPC request with the given method and params.
// Params will be marshaled to JSON. Pass nil for methods without parameters.
func NewRequest(id int, method string, params any) (*Request, error) {
	req := &Request{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		ID:      id,
	}

	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
		req.Params = data
	}

	return req, nil
}

// NewResponse creates a successful JSON-RPC response.
func NewResponse(id int, result any) (*Response, error) {
	resp := &Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
	}

	if result != nil {
		data, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("marshal result: %w", err)
		}
		resp.Result = data
	}

	return resp, nil
}

// NewErrorResponse creates an error JSON-RPC response.
func NewErrorResponse(id int, code int, message string, data any) *Response {
	return &Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// ParseError creates a parse error response (invalid JSON).
func ParseError(id int) *Response {
	return NewErrorResponse(id, ErrCodeParse, "Parse error", nil)
}

// InvalidRequestError creates an invalid request error response.
func InvalidRequestError(id int) *Response {
	return NewErrorResponse(id, ErrCodeInvalidRequest, "Invalid Request", nil)
}

// MethodNotFoundError creates a method not found error response.
func MethodNotFoundError(id int, method string) *Response {
	return NewErrorResponse(id, ErrCodeMethodNotFound, "Method not found", method)
}

// InvalidParamsError creates an invalid params error response.
func InvalidParamsError(id int, details string) *Response {
	return NewErrorResponse(id, ErrCodeInvalidParams, "Invalid params", details)
}

// InternalError creates an internal error response.
func InternalError(id int, err error) *Response {
	return NewErrorResponse(id, ErrCodeInternal, "Internal error", err.Error())
}

// UnauthorizedError creates an unauthorized error response.
func UnauthorizedError(id int) *Response {
	return NewErrorResponse(id, ErrCodeUnauthorized, "Unauthorized", nil)
}

// OperationFailedError creates an operation failed error response.
func OperationFailedError(id int, err error) *Response {
	return NewErrorResponse(id, ErrCodeOperationFailed, "Operation failed", err.Error())
}

// UnmarshalParams extracts and unmarshals request params into the target struct.
func (r *Request) UnmarshalParams(target any) error {
	if r.Params == nil {
		return nil
	}
	return json.Unmarshal(r.Params, target)
}

// UnmarshalResult extracts and unmarshals response result into the target struct.
func (r *Response) UnmarshalResult(target any) error {
	if r.Result == nil {
		return nil
	}
	return json.Unmarshal(r.Result, target)
}

// IsSuccess returns true if the response indicates success (no error).
func (r *Response) IsSuccess() bool {
	return r.Error == nil
}
