package main

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	objDir = ".git/objects"
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
	case "cat-file":
		if len(os.Args) < 3 {
			fmt.Println("usage: mygit cat-file -p <hash>")
			os.Exit(1)
		}

		if os.Args[2] == "-p" {
			hash := os.Args[3]
			b, err := readBlob(hash)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading blob: %s\n", err)
				os.Exit(1)
			}
			fmt.Print(string(b))
		}
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

func readBlob(hash string) ([]byte, error) {
	dirName := hash[:2]
	fileName := hash[2:]
	path := filepath.Join(objDir, dirName, fileName)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create zlib reader: %w", err)
	}
	defer r.Close()

	decompressed, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress data: %w", err)
	}

	return getBlobContent(decompressed), nil
}

func getBlobContent(blob []byte) []byte {
	nullIndex := bytes.IndexByte(blob, 0)
	if nullIndex == -1 {
		return nil
	}
	return blob[nullIndex+1:]
}
