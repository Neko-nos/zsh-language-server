package main

import (
	"flag"
	"fmt"
	"os"

	"zsh-server/internal/lsp"
)

func main() {
	version := flag.Bool("version", false, "Print the server version")
	flag.BoolVar(version, "v", false, "Print the server version")
	flag.Parse()
	if *version {
		fmt.Println("zsh-language-server 0.1.0 (mvdan/sh 3.14.1)")
		return
	}
	os.Exit(lsp.NewServer().Serve(os.Stdin, os.Stdout))
}
