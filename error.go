package jsonrpc

import (
	"fmt"
)

// Error implements a top-level JSON-RPC error.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`

	Data interface{} `json:"data,omitempty"`

	err error
}

// Unwrap returns the embedded error, or nil if none.
func (e Error) Unwrap() error {
	return e.err
}

func (e Error) Error() string {
	return fmt.Sprintf("Error(%d): %s", e.Code, e.Message)
}

// WrapError returns a JSON-RPC Server Error (-32000) embedded and existing
// error that may later be unwrapped. If err is already of Error type, it will
// be returned without any additional wrapping.
func WrapError(err error, data interface{}) *Error {
	// don't wrap error if we already have a JSON-RPC Error
	if rpcErr, ok := err.(*Error); ok {
		return rpcErr
	}

	return &Error{Code: -32000, Message: err.Error(), Data: data, err: err}
}

// ParseError returns a JSON-RPC Parse Error (-32700).
func ParseError(message string, data interface{}) *Error {
	return &Error{Code: -32700, Message: message, Data: data}
}

// InvalidRequest returns a JSON-RPC Invalid Request error (-32600).
func InvalidRequest(message string, data interface{}) *Error {
	return &Error{Code: -32600, Message: message, Data: data}
}

// MethodNotFound returns a JSON-RPC Method Not Found error (-32601).
func MethodNotFound(message string, data interface{}) *Error {
	return &Error{Code: -32601, Message: message, Data: data}
}

// InvalidParams returns a JSON-RPC Invalid Params error (-32602).
func InvalidParams(message string, data interface{}) *Error {
	return &Error{Code: -32602, Message: message, Data: data}
}

// InternalError returns a JSON-RPC Internal Server error (-32603).
func InternalError(message string, data interface{}) *Error {
	return &Error{Code: -32603, Message: message, Data: data}
}

// ServerError returns a JSON-RPC Server Error, which must be given a code
// between -32000 and -32099.
func ServerError(code int, message string, data interface{}) *Error {
	return &Error{Code: code, Message: message, Data: data}
}
