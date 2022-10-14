package main

import (
	"flag"
	"log"
	"os"

	"github.com/gadget-inc/dateilager/pkg/cli"
	"github.com/spf13/cobra/doc"
)

func main() {
	dir := flag.String("doc-path", "./docs/client", "Path directory where you want generated doc files")

	err := os.MkdirAll(*dir, 0755)
	if err != nil {
		log.Fatal(err)
	}

	err = doc.GenMarkdownTree(cli.NewClientCommand(), *dir)
	if err != nil {
		log.Fatal(err)
	}

	err = doc.GenMarkdownTree(cli.NewServerCommand(), *dir)
	if err != nil {
		log.Fatal(err)
	}
}
