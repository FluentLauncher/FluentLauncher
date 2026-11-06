// Package pluginsdk is the stable API for Go launcher plugins. A plugin is
// an executable that serves JSON-RPC over stdin/stdout with 4-byte
// big-endian length frames.
package pluginsdk

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

const MaxMessageBytes = 4 << 20

type Query struct {
	Text         string `json:"text"`
	MaxResults   int    `json:"maxResults"`
	QueryVersion int64  `json:"queryVersion"`
	Keyword      string `json:"keyword,omitempty"`
}

type Action struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type Result struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle"`
	Actions     []Action `json:"actions"`
	PreviewText string   `json:"previewText,omitempty"`
}

type QueryResult struct {
	Results []Result `json:"results"`
}

type ExecuteRequest struct {
	ResultID string `json:"resultId"`
	ActionID string `json:"actionId"`
}

type InitializeResult struct {
	ProtocolVersion int    `json:"protocolVersion"`
	PluginID        string `json:"pluginId"`
}

type Handler interface {
	Query(context.Context, Query) ([]Result, error)
	Execute(context.Context, string, string) error
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve runs the plugin on stdin/stdout until the host closes them.
func Serve(pluginID string, handler Handler) error {
	return ServeIO(pluginID, handler, os.Stdin, os.Stdout)
}

func ServeIO(pluginID string, handler Handler, input io.Reader, output io.Writer) error {
	server := &server{pluginID: pluginID, handler: handler, output: output, active: map[string]context.CancelFunc{}}
	return server.run(input)
}

type server struct {
	pluginID string
	handler  Handler
	output   io.Writer
	writeMu  sync.Mutex
	activeMu sync.Mutex
	active   map[string]context.CancelFunc
	wg       sync.WaitGroup
}

func (s *server) run(input io.Reader) error {
	for {
		payload, err := readFrame(input)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		var req request
		if err := json.Unmarshal(payload, &req); err != nil {
			s.writeError(nil, -32700, "Invalid JSON")
			continue
		}
		if req.Method == "$/cancelRequest" {
			var cancel struct {
				ID json.RawMessage `json:"id"`
			}
			if json.Unmarshal(req.Params, &cancel) == nil {
				s.cancel(string(cancel.ID))
			}
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" {
			s.writeError(req.ID, -32600, "Invalid Request")
			continue
		}
		if len(s.active) >= 8 {
			s.writeError(req.ID, -32000, "Too many requests")
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		key := string(req.ID)
		if key != "" {
			s.activeMu.Lock()
			s.active[key] = cancel
			s.activeMu.Unlock()
		}
		s.wg.Add(1)
		go func(req request, ctx context.Context, cancel context.CancelFunc, key string) {
			defer s.wg.Done()
			defer cancel()
			defer func() {
				if key != "" {
					s.activeMu.Lock()
					delete(s.active, key)
					s.activeMu.Unlock()
				}
			}()
			result, err := s.handle(ctx, req)
			if err != nil {
				s.writeError(req.ID, -32603, err.Error())
				return
			}
			s.writeResult(req.ID, result)
		}(req, ctx, cancel, key)
	}
	s.activeMu.Lock()
	for _, cancel := range s.active {
		cancel()
	}
	s.activeMu.Unlock()
	s.wg.Wait()
	return nil
}

func (s *server) cancel(key string) {
	s.activeMu.Lock()
	cancel := s.active[key]
	s.activeMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *server) handle(ctx context.Context, req request) (any, error) {
	switch req.Method {
	case "initialize":
		return InitializeResult{ProtocolVersion: 1, PluginID: s.pluginID}, nil
	case "ping":
		return "pong", nil
	case "query":
		var query Query
		if err := json.Unmarshal(req.Params, &query); err != nil {
			return nil, fmt.Errorf("unmarshal query: %w", err)
		}
		results, err := s.handler.Query(ctx, query)
		if err != nil {
			return nil, err
		}
		if results == nil {
			results = []Result{}
		}
		return QueryResult{Results: results}, nil
	case "execute":
		var execute ExecuteRequest
		if err := json.Unmarshal(req.Params, &execute); err != nil {
			return nil, fmt.Errorf("unmarshal execute: %w", err)
		}
		if err := s.handler.Execute(ctx, execute.ResultID, execute.ActionID); err != nil {
			return nil, err
		}
		return map[string]any{}, nil
	}
	return nil, fmt.Errorf("unknown method %q", req.Method)
}

func (s *server) writeResult(id json.RawMessage, result any) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = writeJSONFrame(s.output, response{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *server) writeError(id json.RawMessage, code int, message string) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = writeJSONFrame(s.output, response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

func readFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, io.EOF
		}
		return nil, err
	}
	length := int(binary.BigEndian.Uint32(header[:]))
	if length <= 0 || length > MaxMessageBytes {
		return nil, fmt.Errorf("invalid frame length %d", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeJSONFrame(w io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(payload) == 0 || len(payload) > MaxMessageBytes {
		return fmt.Errorf("message exceeds limit")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}
