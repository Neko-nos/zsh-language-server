package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Server struct {
	output      io.Writer
	documents   map[string]*Document
	builtins    map[string]string
	roots       []string
	initialized bool
	shutdown    bool
}

func NewServer() *Server { return &Server{documents: map[string]*Document{}, builtins: builtins()} }

func (s *Server) handle(request request) (any, *rpcError) {
	if request.Method == "initialize" {
		if s.initialized {
			return nil, &rpcError{Code: -32600, Message: "Already initialized"}
		}
		var params struct {
			WorkspaceFolders []struct {
				URI string `json:"uri"`
			} `json:"workspaceFolders"`
		}
		if request.Params != nil {
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return nil, invalidParams(err)
			}
		}
		for _, folder := range params.WorkspaceFolders {
			if path, err := filePath(folder.URI); err == nil {
				s.roots = append(s.roots, path)
			}
		}
		s.initialized = true
		return map[string]any{
			"serverInfo": map[string]string{"name": "zsh-server", "version": "0.1.0"},
			"capabilities": map[string]any{
				"positionEncoding":   "utf-16",
				"textDocumentSync":   map[string]any{"openClose": true, "change": 2},
				"completionProvider": map[string]any{"triggerCharacters": []string{"$", "{"}},
				"hoverProvider":      true, "definitionProvider": true, "referencesProvider": true,
				"documentSymbolProvider": true, "foldingRangeProvider": true,
				"renameProvider": map[string]any{"prepareProvider": true},
			},
		}, nil
	}
	if !s.initialized {
		return nil, &rpcError{Code: -32002, Message: "Server not initialized"}
	}
	if s.shutdown {
		return nil, &rpcError{Code: -32600, Message: "Server has shut down"}
	}
	if request.Method == "shutdown" {
		s.shutdown = true
		return nil, nil
	}
	var params Params
	if request.Params != nil {
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
	}
	uri := params.TextDocument.URI
	switch request.Method {
	case "textDocument/didOpen":
		d := Parse(params.TextDocument)
		s.documents[uri] = d
		return nil, s.publish(d)
	case "textDocument/didChange":
		d, ok := s.documents[uri]
		if !ok {
			return nil, invalidParams(fmt.Errorf("document is not open"))
		}
		if params.TextDocument.Version <= d.Version {
			return nil, nil
		}
		text := d.Text
		for _, edit := range params.ContentChanges {
			if edit.Range == nil {
				text = edit.Text
			} else {
				start, end := d.Offset(edit.Range.Start), d.Offset(edit.Range.End)
				if start > end {
					return nil, invalidParams(fmt.Errorf("invalid edit range"))
				}
				text = text[:start] + edit.Text + text[end:]
			}
			d = Parse(TextDocument{uri, params.TextDocument.Version, text})
		}
		s.documents[uri] = d
		return nil, s.publish(d)
	case "textDocument/didClose":
		delete(s.documents, uri)
		return nil, s.publish(&Document{TextDocument: TextDocument{URI: uri}})
	}
	d := s.documents[uri]
	if d == nil && strings.HasPrefix(request.Method, "textDocument/") {
		return nil, invalidParams(fmt.Errorf("document is not open"))
	}
	switch request.Method {
	case "textDocument/completion":
		return s.completion(d, params.Position), nil
	case "textDocument/hover":
		return s.hover(d, params.Position), nil
	case "textDocument/definition":
		return s.definitions(d, params.Position), nil
	case "textDocument/references":
		return d.references(params.Position, params.Context.IncludeDeclaration), nil
	case "textDocument/documentSymbol":
		return d.Symbols(), nil
	case "textDocument/foldingRange":
		if d.folds == nil {
			return []FoldingRange{}, nil
		}
		return d.folds, nil
	case "textDocument/prepareRename":
		return d.renameRange(params.Position), nil
	case "textDocument/rename":
		if d.renameRange(params.Position) == nil {
			return nil, invalidParams(fmt.Errorf("rename requires a declaration in this document and valid syntax"))
		}
		o := *d.at(params.Position)
		if !o.renameName(params.NewName) {
			return nil, invalidParams(fmt.Errorf("use a valid literal %s name", o.kind))
		}
		refs := d.matching(o, false)
		binding := d.binding(o)
		for _, other := range d.occurrences {
			if other.name != params.NewName || other.kind != o.kind {
				continue
			}
			otherBinding := d.binding(other)
			for _, ref := range refs {
				if otherBinding == binding || other.scope == ref.scope {
					return nil, &rpcError{Code: -32803, Message: "the new name is already used in an affected scope"}
				}
			}
		}
		edits := []TextEdit{}
		for _, ref := range refs {
			edit := TextEdit{d.span(ref.start, ref.end), params.NewName}
			if ref.implicitWidget {
				// Renaming the function must preserve the widget's public name.
				edit = TextEdit{Range{ref.whole.End, ref.whole.End}, " " + params.NewName}
			}
			edits = append(edits, edit)
		}
		return map[string]any{"documentChanges": []any{map[string]any{"textDocument": map[string]any{"uri": uri, "version": d.Version}, "edits": edits}}}, nil
	}
	if request.ID == nil {
		return nil, nil
	}
	return nil, &rpcError{Code: -32601, Message: "Method not found: " + request.Method}
}

func filePath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if u.Scheme != "file" || u.Host != "" && u.Host != "localhost" {
		return "", fmt.Errorf("not a local file URI")
	}
	return u.Path, nil
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func (s *Server) sourceURI(d *Document, src source) string {
	base, err := filePath(d.URI)
	if err != nil {
		return ""
	}
	path := src.path
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[2:])
	}
	roots := append([]string{}, src.directories...)
	for i, root := range roots {
		if !filepath.IsAbs(root) {
			roots[i] = filepath.Join(filepath.Dir(base), root)
		}
	}
	roots = append(roots, filepath.Dir(base))
	for _, root := range s.roots {
		if relative, err := filepath.Rel(root, base); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			roots = append(roots, root)
		}
	}
	if filepath.IsAbs(path) {
		roots = []string{""}
	}
	for _, root := range roots {
		candidate := filepath.Join(root, path)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return fileURI(candidate)
		}
	}
	return ""
}

func (s *Server) related(d *Document) []*Document {
	identity := func(uri string) string {
		if filename, err := filePath(uri); err == nil {
			if real, err := filepath.EvalSymlinks(filename); err == nil {
				return real
			}
		}
		return uri
	}
	result := []*Document{d}
	seen := map[string]bool{identity(d.URI): true}
	for i := 0; i < len(result); i++ {
		for _, src := range result[i].sources {
			uri := s.sourceURI(result[i], src)
			key := identity(uri)
			if uri == "" || seen[key] || len(result) >= 100 {
				continue
			}
			seen[key] = true
			if doc := s.readDocument(uri); doc != nil {
				result = append(result, doc)
			}
		}
	}
	return result
}

func (s *Server) readDocument(uri string) *Document {
	if doc := s.documents[uri]; doc != nil {
		return doc
	}
	path, err := filePath(uri)
	if err != nil {
		return nil
	}
	info, err := os.Stat(path)
	// Keep recursive source discovery bounded while editing.
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return nil
	}
	for openURI, doc := range s.documents {
		if openPath, err := filePath(openURI); err == nil {
			if openInfo, err := os.Stat(openPath); err == nil && os.SameFile(info, openInfo) {
				return doc
			}
		}
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return Parse(TextDocument{URI: uri, Text: string(text)})
}
