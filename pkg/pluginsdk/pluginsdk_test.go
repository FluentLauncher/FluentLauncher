package pluginsdk

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"testing"
)

type handler struct{}

func (handler) Query(ctx context.Context, q Query) ([]Result, error) {
	return []Result{{ID: "echo", Title: q.Text, Subtitle: "s", Actions: []Action{{ID: "copy", Title: "Copy", Kind: "copyText", Value: q.Text}}}}, nil
}

func (handler) Execute(context.Context, string, string) error { return nil }

func TestServeQuery(t *testing.T) {
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- ServeIO("official.echo", handler{}, inputReader, outputWriter) }()

	write(t, inputWriter, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"})
	var initialized InitializeResult
	readResult(t, outputReader, &initialized)
	if initialized.ProtocolVersion != 1 || initialized.PluginID != "official.echo" {
		t.Fatalf("initialize = %+v", initialized)
	}

	write(t, inputWriter, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "query",
		"params": map[string]any{"text": "hi", "maxResults": 5, "queryVersion": 1}})
	var result QueryResult
	readResult(t, outputReader, &result)
	if len(result.Results) != 1 || result.Results[0].Title != "hi" {
		t.Fatalf("query = %+v", result)
	}
	inputWriter.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, w io.Writer, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
}

func readResult(t *testing.T, r io.Reader, target any) {
	t.Helper()
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, binary.BigEndian.Uint32(header[:]))
	if _, err := io.ReadFull(r, payload); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Result json.RawMessage `json:"result"`
		Error  any             `json:"error"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	if response.Error != nil {
		t.Fatalf("error = %v", response.Error)
	}
	if err := json.Unmarshal(response.Result, target); err != nil {
		t.Fatal(err)
	}
}
