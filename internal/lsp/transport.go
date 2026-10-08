package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func invalidParams(err error) *rpcError { return &rpcError{-32602, err.Error()} }

// Serve handles messages in order so every request sees the preceding edits.
func (s *Server) Serve(input io.Reader, output io.Writer) int {
	s.output = output
	reader := bufio.NewReader(input)
	for {
		headers, err := textproto.NewReader(reader).ReadMIMEHeader()
		if err != nil {
			break
		}
		length, err := strconv.Atoi(headers.Get("Content-Length"))
		// Bound allocation for a malformed frame; normal source files are much smaller.
		if err != nil || length < 0 || length > 64*1024*1024 {
			return 1
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(reader, body); err != nil {
			return 1
		}
		var req request
		var result any
		var failure *rpcError
		if !json.Valid(body) {
			failure = &rpcError{-32700, "Invalid JSON"}
		} else if json.Unmarshal(body, &req) != nil || req.JSONRPC != "2.0" || req.Method == "" {
			failure = &rpcError{-32600, "Invalid request"}
		} else {
			if req.Method == "exit" {
				break
			}
			result, failure = s.handle(req)
			if req.ID == nil {
				continue
			}
		}
		response := map[string]any{"id": req.ID}
		if failure != nil {
			response["error"] = failure
		} else {
			response["result"] = result
		}
		if err := s.write(response); err != nil {
			return 1
		}
	}
	if s.shutdown {
		return 0
	}
	return 1
}

func (s *Server) write(message map[string]any) error {
	message["jsonrpc"] = "2.0"
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.output, "Content-Length: %d\r\n\r\n%s", len(body), body)
	return err
}

func (s *Server) publish(d *Document) *rpcError {
	if err := s.write(map[string]any{"method": "textDocument/publishDiagnostics", "params": map[string]any{
		"uri": d.URI, "version": d.Version, "diagnostics": d.Diagnostics(),
	}}); err != nil {
		return &rpcError{-32603, err.Error()}
	}
	return nil
}
