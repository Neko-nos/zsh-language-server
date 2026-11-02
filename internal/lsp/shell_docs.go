package lsp

import (
	"regexp"
	"strings"
)

// Zsh's manual entries start at column 5 in mandoc and column 7 in groff;
// descriptions and examples are indented further.
var manualHeading = regexp.MustCompile(`^ {5,7}([A-Z_][A-Z_0-9]*|[a-z_][a-z_0-9/-]*|[!#$*@?0.:\[-])(?:\s|$)`)
var manualParameterAlias = regexp.MustCompile(`\(([A-Za-z_][A-Za-z_0-9]*)(?: <[A-Z]>)*\)`)

func optionKey(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), "_", "")
}

func (s *Server) shellDocumentation(page, name string) string {
	entries, ok := s.manuals[page]
	if !ok {
		entries = shellManualEntries(manualText(page), page)
		s.manuals[page] = entries
	}
	key, chapter, note := name, "Parameters", ""
	switch page {
	case "zshcompsys":
		chapter = "Completion-System"
	case "zshbuiltins":
		chapter = "Shell-Builtin-Commands"
	case "zshzle":
		chapter = "Zsh-Line-Editor"
	case "zshmodules":
		chapter = "Zsh-Modules"
	case "zshcontrib":
		chapter = "User-Contributions"
	case "zshoptions":
		key, chapter = optionKey(name), "Options"
		if entries[key] == "" && strings.HasPrefix(key, "no") {
			key = strings.TrimPrefix(key, "no")
			if entry := entries[key]; entry != "" {
				note = "`" + name + "` is the negated form of `" + strings.Fields(entry)[0] + "`.\n\n"
			}
		}
	}
	if page == "zshzle" && entries[key] == "" {
		// Some widgets, such as accept-search, are documented inside another widget's entry.
		for _, entry := range entries {
			if nested := nestedManualEntry(entry, name); nested != "" {
				entries[key] = nested
				break
			}
		}
	}
	if entry := entries[key]; entry != "" {
		return note + "```text\n" + entry + "\n```\n\n[Zsh manual](https://zsh.sourceforge.io/Doc/Release/" + chapter + ".html)"
	}
	return ""
}

func nestedManualEntry(text, name string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != name {
			continue
		}
		margin := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		end := i + 1
		for end < len(lines) {
			body := lines[end]
			if strings.TrimSpace(body) != "" && len(body)-len(strings.TrimLeft(body, " \t")) <= len(margin) {
				break
			}
			end++
		}
		for j := i; j < end; j++ {
			lines[j] = strings.TrimPrefix(lines[j], margin)
		}
		return strings.TrimSpace(strings.Join(lines[i:end], "\n"))
	}
	return ""
}

func manualSection(text, start, end string) string {
	_, text, _ = strings.Cut(text, start)
	if end != "" {
		text, _, _ = strings.Cut(text, end)
	}
	return text
}

func shellManualEntries(text, page string) map[string]string {
	entries := map[string]string{}
	switch page {
	case "zshparam":
		text = manualSection(text, "\nPARAMETERS SET BY THE SHELL\n", "")
	case "zshoptions":
		text = manualSection(text, "\nDESCRIPTION OF OPTIONS\n", "\nSINGLE LETTER OPTIONS\n")
	case "zshcompsys":
		entries["compinit"] = "compinit\n" + strings.TrimSpace(manualSection(text, "\n   Use of compinit\n", "\n   Autoloaded files\n"))
		text = manualSection(text, "\n   Functions\n", "\nCOMPLETION SYSTEM CONFIGURATION\n") +
			"\nSTANDARD STYLES\n" + manualSection(text, "\n   Standard Styles\n", "\nBINDABLE COMMANDS\n")
	case "zshbuiltins":
		text = manualSection(text, "\nSHELL BUILTIN COMMANDS\n", "")
	case "zshzle":
		entries["keymaps"] = "KEYMAPS\n" + strings.TrimSpace(manualSection(text, "\nKEYMAPS\n", "\n   Reading Commands\n"))
		addManualEntries(entries, manualSection(text, "\nUSER-DEFINED WIDGETS\n", "\n   Special Widgets\n"), page, "parameter:")
		text = manualSection(text, "\nZLE BUILTINS\n", "\nZLE WIDGETS\n") +
			"\nSTANDARD WIDGETS\n" + manualSection(text, "\nSTANDARD WIDGETS\n", "\nCHARACTER HIGHLIGHTING\n")
	case "zshmodules":
		text = manualSection(text, "\nDESCRIPTION\n", "\nTHE ZSH/ATTR MODULE\n") +
			"\nTHE ZSH/ZUTIL MODULE\n" + manualSection(text, "\nTHE ZSH/ZUTIL MODULE\n", "")
	case "zshcontrib":
		recent := manualSection(text, "\nREMEMBERING RECENT DIRECTORIES\n", "\nABBREVIATED DYNAMIC REFERENCES TO DIRECTORIES\n")
		addManualEntries(entries, manualSection(recent, "\n   Configuration\n", "\n   Use with dynamic directory naming\n"), page, "chpwd:")
		vcs := manualSection(text, "This is a description of all styles that are looked up.\n", "The default values for these styles in all contexts are:")
		addManualEntries(entries, vcs, page, "vcs_info:")
		return entries
	}
	addManualEntries(entries, text, page, "")
	return entries
}

func addManualEntries(entries map[string]string, text, page, prefix string) {
	completers := false
	var names, lines []string
	flush := func() {
		if len(lines) > 0 {
			margin := lines[0][:len(lines[0])-len(strings.TrimLeft(lines[0], " "))]
			for i, line := range lines {
				lines[i] = strings.TrimPrefix(line, margin)
			}
		}
		entry := strings.TrimSpace(strings.Join(lines, "\n"))
		for _, name := range names {
			entries[prefix+name] = entry
		}
		names, lines = nil, nil
	}
	for _, line := range strings.Split(text, "\n") {
		if page == "zshcompsys" && line == "CONTROL FUNCTIONS" {
			flush()
			completers = true
		}
		if heading := manualHeading.FindStringSubmatch(line); heading != nil {
			if completers && !strings.HasPrefix(heading[1], "_") {
				continue
			}
			// Adjacent headings share a description (for example PROMPT through PROMPT4).
			if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
				flush()
			}
			name := heading[1]
			if page == "zshoptions" {
				name = optionKey(name)
			} else if page == "zshparam" {
				for _, alias := range manualParameterAlias.FindAllStringSubmatch(line, -1) {
					names = append(names, alias[1])
				}
			}
			names = append(names, name)
		} else if strings.TrimSpace(line) != "" && len(line)-len(strings.TrimLeft(line, " \t")) < 5 && !strings.HasPrefix(line, "\t") {
			flush()
		}
		if len(names) > 0 {
			lines = append(lines, line)
		}
	}
	flush()
}
