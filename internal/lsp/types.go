package lsp

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type Markup struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type Hover struct {
	Contents Markup `json:"contents"`
	Range    Range  `json:"range"`
}

type Completion struct {
	Label         string   `json:"label"`
	Kind          int      `json:"kind"`
	Detail        string   `json:"detail,omitempty"`
	Documentation *Markup  `json:"documentation,omitempty"`
	TextEdit      TextEdit `json:"textEdit"`
}

type Symbol struct {
	Name           string `json:"name"`
	Kind           int    `json:"kind"`
	Range          Range  `json:"range"`
	SelectionRange Range  `json:"selectionRange"`
}

type FoldingRange struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
}

type TextDocument struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
	Text    string `json:"text"`
}

type Params struct {
	ContentChanges []struct {
		Range *Range `json:"range"`
		Text  string `json:"text"`
	} `json:"contentChanges"`
	TextDocument TextDocument `json:"textDocument"`
	Position     Position     `json:"position"`
	NewName      string       `json:"newName"`
	Context      struct {
		IncludeDeclaration bool `json:"includeDeclaration"`
	} `json:"context"`
}
