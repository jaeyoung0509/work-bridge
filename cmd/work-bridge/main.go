package main

import (
	"context"
	"os"

	"github.com/jaeyoung0509/work-bridge/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
