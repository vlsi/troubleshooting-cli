package main

import (
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/vlsi/troubleshooting-cli/internal/app"
	"github.com/vlsi/troubleshooting-cli/internal/mcp"
	"github.com/vlsi/troubleshooting-cli/internal/storage"
)

func main() {
	dbPath, err := storage.DefaultDBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "db path: %v\n", err)
		os.Exit(1)
	}
	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	svc := app.NewService(store, func() string { return uuid.New().String() })
	mcp.NewServer(svc).Run(os.Stdin, os.Stdout)
}
