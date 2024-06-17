package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

func init() {
	textHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				source := a.Value.Any().(*slog.Source)
				source.File = filepath.Base(source.File)
			}
			return a
		},
	})

	logger := slog.New(textHandler)

	slog.SetDefault(logger)
}

// Usage: your_git.sh <command> <arg1> <arg2> ...
func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: mygit <command> [<args>...]")
		os.Exit(1)
	}

	switch command := os.Args[1]; command {
	case "init":
		initRepo()
	default:
		slog.Error("Unknown command", slog.String("command", command))
		os.Exit(1)
	}
}

func initRepo() {
	for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory: %s\n", err)
		}
	}

	headFileContents := []byte("ref: refs/heads/main\n")
	if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %s\n", err)
	}

	fmt.Println("Initialized git directory")
}
