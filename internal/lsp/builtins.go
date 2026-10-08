package lsp

import "strings"

func builtins() map[string]string {
	return map[string]string{
		"alias":      "alias [name[=value] ...]\nDefine or list command aliases.",
		"autoload":   "autoload [-Uz] name ...\nMark functions for loading from $fpath on their first call. -U suppresses alias expansion; -z selects Zsh style.",
		"bg":         "bg [job ...]\nResume suspended jobs in the background.",
		"bindkey":    "bindkey [options] key [widget]\nBind keys to Zsh line editor widgets.",
		"builtin":    "builtin command [argument ...]\nRun a shell builtin even if a function has the same name.",
		"cd":         "cd [directory]\nChange the current directory.",
		"command":    "command command [argument ...]\nRun a command without shell function lookup.",
		"compdef":    "compdef function command ...\nAssociate a completion function with commands after compinit.",
		"compinit":   "autoload -Uz compinit; compinit\nInitialize Zsh's completion system.",
		"declare":    "declare [options] [name[=value] ...]\nSet parameter attributes; a synonym for typeset.",
		"dirs":       "dirs [options]\nDisplay or modify the directory stack.",
		"disown":     "disown [job ...]\nRemove jobs from the shell's job table.",
		"echo":       "echo [arguments ...]\nWrite arguments followed by a newline. Use print or printf for explicit formatting.",
		"emulate":    "emulate [-LR] zsh\nSelect shell emulation. With -L in a function, changes to options, patterns and traps are local.",
		"eval":       "eval [argument ...]\nJoin arguments and execute them as shell code.",
		"exec":       "exec command [argument ...]\nReplace the shell with a command.",
		"exit":       "exit [status]\nExit the shell with the given status.",
		"export":     "export [name[=value] ...]\nMark parameters for inclusion in the environment of commands.",
		"fg":         "fg [job ...]\nResume jobs in the foreground.",
		"functions":  "functions [name ...]\nDisplay or modify shell function definitions.",
		"hash":       "hash [name=path ...]\nManage the command hash table.",
		"jobs":       "jobs [options] [job ...]\nList active jobs.",
		"kill":       "kill [-signal] process-or-job ...\nSend a signal to a process or job.",
		"let":        "let expression ...\nEvaluate arithmetic expressions.",
		"local":      "local [options] [name[=value] ...]\nDeclare parameters local to a function.",
		"popd":       "popd [options]\nRemove an entry from the directory stack and change directory.",
		"print":      "print [-r] [--] argument ...\nWrite arguments. -r treats backslashes literally; -- ends option processing.",
		"printf":     "printf format [argument ...]\nWrite formatted output.",
		"pushd":      "pushd [directory]\nChange directory and push it onto the directory stack.",
		"pwd":        "pwd [-LP]\nPrint the current working directory.",
		"read":       "read [options] [name ...]\nRead input into shell parameters.",
		"readonly":   "readonly [name[=value] ...]\nDeclare parameters that cannot be changed.",
		"rehash":     "rehash\nRebuild the command hash table.",
		"return":     "return [status]\nReturn from a function or sourced script.",
		"set":        "set [options] [-- argument ...]\nSet shell options or positional parameters.",
		"setopt":     "setopt [option ...]\nEnable shell options.",
		"shift":      "shift [count] [name ...]\nRemove leading positional parameters or array elements.",
		"source":     "source file [argument ...]\nRead and execute a file in the current shell.",
		".":          ". file [argument ...]\nRead and execute a file in the current shell.",
		"test":       "test expression\nEvaluate a conditional expression; [[ ... ]] offers Zsh's extended syntax.",
		"trap":       "trap [action] [signal ...]\nSet handlers for signals or shell events.",
		"true":       "true\nReturn a successful exit status.",
		"false":      "false\nReturn an unsuccessful exit status.",
		"typeset":    "typeset [-aAgi] [name[=value] ...]\nDeclare parameters and attributes. -a creates an array, -A an associative array, -g selects global scope, and -i integer arithmetic.",
		"unalias":    "unalias name ...\nRemove aliases.",
		"unfunction": "unfunction name ...\nRemove shell functions.",
		"unset":      "unset name ...\nRemove parameters.",
		"unsetopt":   "unsetopt [option ...]\nDisable shell options.",
		"wait":       "wait [process-or-job ...]\nWait for child processes or jobs.",
		"whence":     "whence [options] name ...\nDescribe how Zsh would resolve a command name.",
		"zcompile":   "zcompile [options] file [name ...]\nCompile shell code for faster loading.",
		"zle":        "zle [options] [widget]\nCreate or invoke Zsh line editor widgets.",
		"zmodload":   "zmodload [options] [module ...]\nLoad or inspect Zsh modules.",
		"zparseopts": "zparseopts [options] specification ...\nParse command options (requires zsh/zutil).",
		"zstyle":     "zstyle pattern style [string ...]\nConfigure styles used by completion and other Zsh functions.",
	}
}

func builtinDocumentation(help string) *Markup {
	parts := strings.SplitN(help, "\n", 2)
	return &Markup{Kind: "markdown", Value: "```zsh\n" + parts[0] + "\n```\n\n" + parts[1] + "\n\n[Zsh manual](https://zsh.sourceforge.io/Doc/Release/Shell-Builtin-Commands.html)"}
}
