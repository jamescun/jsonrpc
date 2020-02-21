package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
)

// Request contains the parameters of an incoming JSON-RPC request.
type Request struct {
	Version string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`

	ctx   context.Context
	raddr string
}

// Context returns the execution context of Request.
func (r *Request) Context() context.Context {
	if r.ctx == nil {
		return context.Background()
	}

	return r.ctx
}

// WithContext returns a shallow copy of Request with the given context.
func (r *Request) WithContext(ctx context.Context) *Request {
	x := r.Clone()
	x.ctx = ctx
	return x
}

// Clone returns a shallow copy of Request r.
func (r *Request) Clone() *Request {
	return &Request{
		Version: r.Version,
		Method:  r.Method,
		Params:  r.Params,
		ID:      r.ID,

		ctx:   r.ctx,
		raddr: r.raddr,
	}
}

// RemoteAddr returns the ip:port of the remote client.
func (r *Request) RemoteAddr() string {
	return r.raddr
}

type response struct {
	Version string          `json:"jsonrpc"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
}

// ResponseWriter is implemented by each server type to handle how responses
// are marshalled to the client.
type ResponseWriter interface {
	// Write sends a response to the client. Depending on the server type,
	// this may be called multiple times to write multiple responses.
	//
	// If given an error, the writer will marshal the response as the Error
	// key, otherwise the Result key.
	Write(interface{}) error
}

// Handler is implemented by anything that can execute a JSON-RPC request.
type Handler interface {
	ServeJSONRPC(ResponseWriter, *Request)
}

// HandlerFunc adapts a function to implement the Handler interface.
type HandlerFunc func(ResponseWriter, *Request)

// ServeJSONRPC calls hf(w, r).
func (hf HandlerFunc) ServeJSONRPC(w ResponseWriter, r *Request) {
	hf(w, r)
}

const (
	// MIMEType is the expected Content-Type supplied by the client.
	MIMEType = "application/json"

	// ContentType is the contents of the Content-Type header sent by the
	// server.
	ContentType = MIMEType + "; charset=utf-8"
)

type httpResponseWriter struct {
	w   http.ResponseWriter
	req *Request
	mu  sync.Mutex
}

func (h *httpResponseWriter) Write(r interface{}) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	res := &response{Version: "2.0", ID: h.req.ID}

	if err, ok := r.(error); ok {
		res.Error = WrapError(err, nil)
	} else {
		res.Result = r
	}

	return json.NewEncoder(h.w).Encode(res)
}

// HTTP adapts a JSON-RPC Handler into a HTTP handler for the request-response
// pattern.
func HTTP(h Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if hdr := r.Header.Get("Content-Type"); !strings.HasPrefix(hdr, MIMEType) {
			http.Error(w, fmt.Sprintf("unsupported content type %q", hdr), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", ContentType)

		req := &Request{ctx: r.Context(), raddr: r.RemoteAddr}
		res := &httpResponseWriter{w: w, req: req}

		err := json.NewDecoder(r.Body).Decode(req)
		if err != nil {
			res.Write(ParseError(err.Error(), nil))
			return
		}

		if req.Version != "2.0" {
			res.Write(InvalidRequest("expected JSON-RPC 2.0", nil))
			return
		}

		h.ServeJSONRPC(res, req)
	})
}

type method struct {
	name       string
	fnV        reflect.Value
	reqT, resT reflect.Type
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()
var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

func reflectMethod(fn interface{}) (*method, error) {
	fnV := reflect.ValueOf(fn)
	fnT := fnV.Type()

	if fnT.Kind() != reflect.Func {
		return nil, fmt.Errorf("expected function, got %s", fnT.Kind())
	}

	if fnT.NumIn() < 1 {
		return nil, fmt.Errorf("function must accept at least a context.Context")
	} else if fnT.NumIn() > 2 {
		return nil, fmt.Errorf("function may only accept context.Context and a request type")
	} else if fnT.NumOut() < 1 {
		return nil, fmt.Errorf("function must at least return an error")
	} else if fnT.NumOut() > 2 {
		return nil, fmt.Errorf("function may only return a response type and an error")
	}

	if !fnT.In(0).Implements(contextType) {
		return nil, fmt.Errorf("first argument must implement context.Context, got %s", fnT.In(0))
	} else if !fnT.Out(fnT.NumOut() - 1).Implements(errorType) {
		return nil, fmt.Errorf("second return must implement error, got %s", fnT.Out(fnT.NumOut()-1))
	}

	m := &method{name: fnT.Name(), fnV: fnV}

	if fnT.NumIn() == 2 {
		reqT := fnT.In(1)
		if reqT.Kind() != reflect.Ptr || reqT.Elem().Kind() != reflect.Struct {
			return nil, fmt.Errorf("second argument must be struct pointer, got %s", reqT)
		}

		m.reqT = reqT.Elem()
	}

	if fnT.NumOut() == 2 {
		resT := fnT.Out(0)
		if resT.Kind() != reflect.Ptr || resT.Elem().Kind() != reflect.Struct {
			return nil, fmt.Errorf("first return must be struct pointer, got %s", resT)
		}

		m.resT = resT.Elem()
	}

	return m, nil
}

// Service is a collection of JSON-RPC methods.
type Service struct {
	hn map[string]*method
}

// RegisterableService is implemented by services that can register themselves
// with a Service object.
type RegisterableService interface {
	Register(*Service)
}

// NewService creates a new Service assignment, optionally for a service that
// can self register its methods.
func NewService(rs RegisterableService) *Service {
	s := &Service{
		hn: make(map[string]*method),
	}

	if rs != nil {
		rs.Register(s)
	}

	return s
}

// Register reflects a function and registers it as a method on Service s
// identified by the given name.
//
// This method will panic if given a function whose signature is not:
//   func(context.Context) error
//   func(context.Context, *struct) error
//   func(context.Context) (*struct, error)
//   func(context.Context, *struct) (*struct, error)
func (s *Service) Register(name string, fn interface{}) {
	m, err := reflectMethod(fn)
	if err != nil {
		panic(err)
	}

	if s.hn == nil {
		s.hn = make(map[string]*method)
	} else {
		_, ok := s.hn[name]
		if ok {
			panic("method already registered")
		}
	}

	s.hn[name] = m
}

// ServeJSONRPC routes based on method name to a registered handler.
func (s *Service) ServeJSONRPC(w ResponseWriter, r *Request) {
	ctx := r.Context()

	m, ok := s.hn[r.Method]
	if !ok {
		w.Write(MethodNotFound("method not found", nil))
		return
	}

	in := []reflect.Value{reflect.ValueOf(ctx)}

	if m.reqT != nil {
		req := reflect.New(m.reqT)

		err := json.Unmarshal(r.Params, req.Interface())
		if err != nil {
			w.Write(ParseError(err.Error(), nil))
			return
		}

		in = append(in, req)
	}

	out := m.fnV.Call(in)

	if !out[len(out)-1].IsNil() {
		// this will always implement error, otherwise validation is wrong
		err := out[len(out)-1].Interface().(error)

		w.Write(err)
		return
	}

	if len(out) == 2 {
		w.Write(out[0].Interface())
	}
}
