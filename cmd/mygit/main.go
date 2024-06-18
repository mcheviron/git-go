package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

const (
	objDir = ".git/objects"
)

var ignoredDirs = []string{".", "..", ".git"}

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
		if err := initRepo(); err != nil {
			slog.Error("Failed to initialize repo", "err", err)
			os.Exit(1)
		}
	case "cat-file":
		if len(os.Args) < 3 {
			fmt.Println("usage: mygit cat-file -p <hash>")
			os.Exit(1)
		}

		if os.Args[2] == "-p" {
			hash := os.Args[3]
			b, err := readBlob(hash)
			if err != nil {
				slog.Error("Error reading blob", "err", err)
				os.Exit(1)
			}
			fmt.Print(string(b))
		}
	case "hash-object":
		if len(os.Args) < 3 {
			fmt.Println("usage: mygit hash-object [-w] <file>")
			os.Exit(1)
		}
		file := os.Args[len(os.Args)-1]
		objectContent, hash, err := hashObject(file)
		if err != nil {
			slog.Error("Error hashing object", "err", err)
			os.Exit(1)
		}

		if len(os.Args) > 3 && os.Args[2] == "-w" {
			if err := writeObject(objectContent, hash); err != nil {
				slog.Error("Error writing object", "err", err)
				os.Exit(1)
			}
		}

		fmt.Printf("%x\n", hash)
	case "ls-tree":
		if len(os.Args) < 3 {
			fmt.Println("usage: mygit ls-tree [--name-only] <hash>")
			os.Exit(1)
		}
		var hexHash string
		nameOnly := false
		if os.Args[2] == "--name-only" {
			nameOnly = true
			hexHash = os.Args[3]
		} else {
			hexHash = os.Args[2]
		}
		treeEntries, err := lsTree(hexHash, nameOnly)
		if err != nil {
			slog.Error("Error listing tree", "err", err)
			os.Exit(1)
		}
		for _, entry := range treeEntries {
			fmt.Println(entry)
		}
	case "write-tree":
		if len(os.Args) < 2 {
			fmt.Println("usage: mygit write-tree")
			os.Exit(1)
		}
		hash, err := writeTree(".")
		if err != nil {
			slog.Error("Error writing tree", "err", err)
			os.Exit(1)
		}
		fmt.Printf("%x\n", hash)

	default:
		slog.Error("Unknown command", slog.String("command", command))
		os.Exit(1)
	}
}

func initRepo() error {
	for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("error creating directory: %w", err)
		}
	}

	headFileContents := []byte("ref: refs/heads/main\n")
	if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
		return fmt.Errorf("error writing file: %w", err)
	}

	fmt.Println("Initialized git directory")
	return nil
}

func readBlob(hash string) ([]byte, error) {
	path := filepath.Join(objDir, hash[:2], hash[2:])

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

func hashObject(filePath string) (string, [20]byte, error) {
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return "", [20]byte{}, fmt.Errorf("failed to read file: %v", err)
	}

	objectContent := fmt.Sprintf("blob %d\x00%s", len(fileContent), fileContent)

	hash := sha1.Sum([]byte(objectContent))
	return objectContent, hash, nil
}

func writeObject(objectContent string, hash [20]byte) error {
	hexHash := fmt.Sprintf("%x", hash)
	path := filepath.Join(objDir, hexHash[:2], hexHash[2:])

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create object directory: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create object file: %w", err)
	}
	defer f.Close()

	w := zlib.NewWriter(f)
	defer w.Close()

	if _, err := w.Write([]byte(objectContent)); err != nil {
		return fmt.Errorf("failed to compress object content: %w", err)
	}

	return nil
}

func lsTree(hexHash string, nameOnly bool) ([]string, error) {
	// tree <size>\0
	// <mode> <name>\0<20_byte_sha>
	// <mode> <name>\0<20_byte_sha>
	path := filepath.Join(objDir, hexHash[:2], hexHash[2:])
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

	if !bytes.HasPrefix(decompressed, []byte("tree")) {
		return nil, fmt.Errorf("object is not a tree")
	}

	nullIndex := bytes.IndexByte(decompressed, 0)
	if nullIndex == -1 {
		return nil, fmt.Errorf("invalid tree object format")
	}
	content := decompressed[nullIndex+1:]

	var result []string
	for len(content) > 0 {
		nullIndex = bytes.IndexByte(content, 0)
		if nullIndex == -1 {
			break
		}

		entry := content[:nullIndex]
		content = content[nullIndex+1:]

		parts := bytes.Split(entry, []byte(" "))
		mode := string(parts[0])
		name := string(parts[1])

		sha := content[:20]
		content = content[20:]

		if nameOnly {
			result = append(result, fmt.Sprintf("%s", name))
		} else {
			result = append(result, fmt.Sprintf("%s %s %x", mode, name, sha))
		}
	}

	sort.Strings(result)
	return result, nil
}

func writeTree(path string) ([20]byte, error) {
	// tree <size>\0
	// <mode> <name>\0<20_byte_sha>
	// <mode> <name>\0<20_byte_sha>
	var treeEntries [][]byte

	slog.Info("Reading directory", "path", path)
	entries, err := os.ReadDir(path)
	if err != nil {
		slog.Error("Failed to read directory", "error", err)
		return [20]byte{}, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(path, entry.Name())
		if slices.Contains(ignoredDirs, entry.Name()) {
			slog.Info("Ignoring directory", "path", entryPath)
			continue
		}

		var mode string
		var hash [20]byte

		if entry.IsDir() {
			slog.Info("Processing directory", "path", entryPath)
			mode = "40000"
			hash, err = writeTree(entryPath)
			if err != nil {
				slog.Error("Failed to write tree object", "path", entryPath, "error", err)
				return [20]byte{}, fmt.Errorf("failed to write tree object: %w", err)
			}
		} else {
			slog.Info("Processing file", "path", entryPath)
			_, hash, err = hashObject(entryPath)
			if err != nil {
				slog.Error("Failed to hash object", "path", entryPath, "error", err)
				return [20]byte{}, fmt.Errorf("failed to hash object: %w", err)
			}

			mode = "100644"
		}

		entryData := []byte(fmt.Sprintf("%s %s\x00", mode, filepath.Base(entryPath)))
		entryData = append(entryData, hash[:]...)
		treeEntries = append(treeEntries, entryData)
	}

	// Sort the tree entries
	sort.Slice(treeEntries, func(i, j int) bool {
		return bytes.Compare(treeEntries[i], treeEntries[j]) < 0
	})

	// Flatten the sorted tree entries
	var flattenedTreeEntries []byte
	for _, entry := range treeEntries {
		flattenedTreeEntries = append(flattenedTreeEntries, entry...)
	}

	treeObject := fmt.Sprintf("tree %d\x00%s", len(flattenedTreeEntries), flattenedTreeEntries)
	hash := sha1.Sum([]byte(treeObject))

	slog.Info("Writing tree object", "hash", fmt.Sprintf("%x", hash))
	if err := writeObject(treeObject, hash); err != nil {
		slog.Error("Failed to write tree object", "error", err)
		return [20]byte{}, fmt.Errorf("failed to write tree object: %w", err)
	}

	slog.Info("Tree object written successfully", "hash", fmt.Sprintf("%x", hash))
	return hash, nil
}
