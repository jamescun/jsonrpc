# JSONRPC

JSONRPC is a server implementation of the JSON-RPC 2.0 specification in Go.

Currently, only HTTP is supported as a transport. HTTP/2 is recommended for performance reasons.


## Installation

```sh
go get -u github.com/jamescun/jsonrpc
```


## Examples

### Catch-All Handler

```go
type HelloWorld struct {}

type GreetRequest struct {
	Name string `json:"name"`
}

type GreetResponse struct {
	Greeting string `json:"greeting"`
}

func (hw *HelloWorld) ServeJSONRPC(w jsonrpc.ResponseWriter, r *jsonrpc.Request) {
	switch r.Method {
	case "Greet":
		var req GreetRequest
		err := json.Unmarshal(r.Params, &req)
		if err != nil {
			w.Write(jsonrpc.ParseError(err.Error(), nil))
			return
		}

		w.Write(&GreetResponse{Greeting: "Hello " + req.Name + "!"})

	default:
		w.Write(jsonrpc.MethodNotFound("method not found", nil))
	}
}

func main() {
	hw := &HelloWorld{}

	s := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: jsonrpc.HTTP(hw),
	}

	s.ListenAndServe()
}
```

### Service from Reflected Methods

```go
type HelloWorld struct {}

func (hw *HelloWorld) Register(svc *jsonrpc.Service) {
	svc.Register("Greet", hw.Greet)
}

type GreetRequest struct {
	Name string `json:"name"`
}

type GreetResponse struct {
	Greeting string `json:"greeting"`
}

func (hw *HelloWorld) Greet(ctx context.Context, req *GreetRequest) (*GreetResponse, error) {
	return &GreetResponse{
		Greeting: "Hello " + req.Name + "!",
	}, nil
}

func main() {
	hw := &HelloWorld{}
	svc := jsonrpc.NewService(hw)

	s := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: jsonrpc.HTTP(svc),
	}

	s.ListenAndServe()
}
```
