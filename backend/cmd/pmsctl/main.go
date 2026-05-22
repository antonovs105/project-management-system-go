package main

import (
	"context"
	"os"

	"github.com/antonovs105/project-management-system-go/internal/pmsctl"
)

// main runs the pmsctl maintenance CLI.
func main() {
	os.Exit(pmsctl.NewRunner().Run(context.Background(), os.Args[1:]))
}
