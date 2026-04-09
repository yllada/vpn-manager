// Package protocol provides JSON-RPC 2.0 encoding and decoding utilities.
package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Codec handles JSON-RPC encoding and decoding over a connection.
// It uses newline-delimited JSON for message framing.
// Codec is safe for concurrent use from multiple goroutines.
type Codec struct {
	conn io.ReadWriteCloser

	// Separate mutexes for read and write to allow concurrent operations
	readMu  sync.Mutex
	writeMu sync.Mutex

	reader *bufio.Reader
	closed bool
}

// NewCodec creates a new codec wrapping the given connection.
func NewCodec(conn io.ReadWriteCloser) *Codec {
	return &Codec{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

// WriteRequest encodes and writes a request to the connection.
func (c *Codec) WriteRequest(req *Request) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.closed {
		return ErrConnectionClosed
	}

	return c.writeMessage(req)
}

// WriteResponse encodes and writes a response to the connection.
func (c *Codec) WriteResponse(resp *Response) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.closed {
		return ErrConnectionClosed
	}

	return c.writeMessage(resp)
}

// writeMessage marshals and writes a message with newline delimiter.
// Must be called with writeMu held.
func (c *Codec) writeMessage(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	// Append newline delimiter
	data = append(data, '\n')

	_, err = c.conn.Write(data)
	if err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	return nil
}

// ReadRequest reads and decodes a request from the connection.
func (c *Codec) ReadRequest() (*Request, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	if c.closed {
		return nil, ErrConnectionClosed
	}

	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		if err == io.EOF {
			return nil, ErrConnectionClosed
		}
		return nil, fmt.Errorf("read request: %w", err)
	}

	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return nil, fmt.Errorf("unmarshal request: %w", err)
	}

	// Validate JSON-RPC version
	if req.JSONRPC != JSONRPCVersion {
		return nil, fmt.Errorf("invalid jsonrpc version: %s", req.JSONRPC)
	}

	return &req, nil
}

// ReadResponse reads and decodes a response from the connection.
func (c *Codec) ReadResponse() (*Response, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	if c.closed {
		return nil, ErrConnectionClosed
	}

	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		if err == io.EOF {
			return nil, ErrConnectionClosed
		}
		return nil, fmt.Errorf("read response: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Validate JSON-RPC version
	if resp.JSONRPC != JSONRPCVersion {
		return nil, fmt.Errorf("invalid jsonrpc version: %s", resp.JSONRPC)
	}

	return &resp, nil
}

// Close closes the underlying connection.
func (c *Codec) Close() error {
	c.writeMu.Lock()
	c.readMu.Lock()
	defer c.writeMu.Unlock()
	defer c.readMu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	return c.conn.Close()
}

// IsClosed returns true if the codec has been closed.
func (c *Codec) IsClosed() bool {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	return c.closed
}

// Encoder writes JSON-RPC messages to a writer.
// Use this for one-way encoding when you don't need the full Codec.
type Encoder struct {
	w  io.Writer
	mu sync.Mutex
}

// NewEncoder creates a new encoder writing to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes a JSON-RPC message with newline delimiter.
func (e *Encoder) Encode(msg any) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	data = append(data, '\n')
	_, err = e.w.Write(data)
	return err
}

// Decoder reads JSON-RPC messages from a reader.
// Use this for one-way decoding when you don't need the full Codec.
type Decoder struct {
	r  *bufio.Reader
	mu sync.Mutex
}

// NewDecoder creates a new decoder reading from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

// DecodeRequest reads and decodes a Request.
func (d *Decoder) DecodeRequest() (*Request, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	line, err := d.r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return nil, err
	}

	return &req, nil
}

// DecodeResponse reads and decodes a Response.
func (d *Decoder) DecodeResponse() (*Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	line, err := d.r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
