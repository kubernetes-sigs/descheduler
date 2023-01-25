package main

import (
	"log"
	"os"

	"github.com/spf13/cobra/doc"
	"sigs.k8s.io/descheduler/cmd/descheduler/app"
)

var docGenPath = "docs/cli"

func main() {
	cmd := app.NewDeschedulerCommand(os.Stdout)
	cmd.AddCommand(app.NewVersionCommand())
	cmd.DisableAutoGenTag = true // Disable this so that the diff wont track it
	if err := doc.GenMarkdownTree(cmd, docGenPath); err != nil {
		log.Fatal(err)
	}
}
