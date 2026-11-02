package lsp

import "testing"

func TestStyleHover(t *testing.T) {
	s := NewServer()
	for _, test := range []struct{ text, word, description string }{
		{"zstyle ':completion:*:default' menu select=2", "':completion:*:default'", ":completion:function:completer:command:argument:tag"},
		{"zstyle ':completion:*' matcher-list 'm:{a-z}={A-Z}'", "matcher-list", "list of match specifications"},
		{"zstyle ':completion:*' matcher-list 'm:{a-z}={A-Z}'", "'m:{a-z}={A-Z}'", "case-insensitive completion"},
		{"zstyle ':completion:*:default' menu select=1", "menu", "select=num"},
		{"zstyle ':completion:*:default' menu select=1", "select=1", "select=num"},
		{"zstyle ':completion:*' completer _complete", "completer", "_complete _ignored"},
		{"zstyle ':completion:*' completer _complete:sample", "_complete:sample", "normal completion behaviour"},
		{"zstyle ':completion:*' completer _complete _ignored", "_ignored", "ignored-patterns"},
		{"zstyle ':completion:*:(*files|*directories)' ignored-patterns \\\n    '(|*/)(dummy-cache|dummy-output)(|/)'", "ignored-patterns", "excluded from consideration"},
		{"zstyle ':completion:*:(*files|*directories)' ignored-patterns \\\n    '(|*/)(dummy-cache|dummy-output)(|/)'", "'(|*/)(dummy-cache|dummy-output)(|/)'", "excluded from consideration"},
		{"builtin zstyle -- ':completion:*' 'matcher-list' ''", "'matcher-list'", "list of match specifications"},
		{"zstyle -e ':completion:*' menu 'reply=(select=2)'", "menu", "select=num"},
		{"zstyle -e ':completion:*' menu 'reply=(select=2)'", "'reply=(select=2)'", ""},
		{"zstyle -s ':completion:*' menu sample_output", "menu", "select=num"},
		{"zstyle -s ':completion:*' menu sample_output", "sample_output", ""},
		{"zstyle -g sample_output ':completion:*' menu", "menu", "select=num"},
		{"zstyle -d ':completion:*' menu matcher-list", "matcher-list", "list of match specifications"},
		{"zstyle ':sample:*' menu select=2", "menu", ""},
		{"zstyle ':completion:*' sample-style dummy-value", "sample-style", ""},
		{"zstyle ':vcs_info:*' check-for-changes true", "check-for-changes", "uncommitted changes"},
		{"zstyle ':vcs_info:*' check-for-changes true", "true", "uncommitted changes"},
		{"zstyle ':vcs_info:*' enable git", "':vcs_info:*'", ":vcs_info:vcs-string:user-context:repo-root-name"},
		{"zstyle ':chpwd:*' recent-dirs-default false", "recent-dirs-default", "expecting a recent directory index"},
		{"zstyle ':chpwd:*' recent-dirs-max 42", "42", "maximum number of directories"},
		{"zstyle ':chpwd:*' recent-dirs-file /dummy/directories", "/dummy/directories", "file where the list of directories is saved"},
		{"zstyle ':completion:*' recent-dirs-insert fallback", "fallback", "recent directories will be tried first"},
	} {
		t.Run(test.text+"/"+test.word, func(t *testing.T) {
			assertSettingHover(t, s, test.text, test.word, test.description)
		})
	}
}
