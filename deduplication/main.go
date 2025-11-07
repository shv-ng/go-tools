package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	start := time.Now()
	flag.Parse()
	if len(flag.Args()) > 1 {
		return errors.New("Usage:\n deduplication [path]\t\tdefault .")
	}

	root, err := os.Getwd()
	if err != nil {
		return err
	}

	if len(flag.Args()) == 1 {
		root = flag.Arg(0)
	}

	g := new(errgroup.Group)
	ch := make(chan string)
	var ops, sz atomic.Uint64
	var m sync.Map

	walk(root, ch, g, &ops, &sz)
	hashify(ch, g, &m)

	if err := g.Wait(); err != nil {
		return err
	}

	foundAny := false
	m.Range(func(key, value any) bool {
		paths, ok := value.(*[]string)
		if !ok || len(*paths) <= 1 {
			return true
		}

		if !foundAny {
			fmt.Println("Duplicate files found:")
			foundAny = true
		}
		fmt.Printf("\nHash: %s\n", key)
		for _, p := range *paths {
			fmt.Printf("  %s\n", p)
		}
		return true
	})

	if !foundAny {
		fmt.Println("All files are unique")
	}

	mb := sz.Load() / 1024 / 1024
	kb := sz.Load()/1024 - mb*1024
	fmt.Printf("\nFile scanned: %d\n", ops.Load())
	fmt.Printf("Total files size sum: %d.%d MB\n", mb, kb)
	fmt.Printf("Time taken: %v\n", time.Since(start))
	return nil
}

func walk(root string, ch chan<- string, g *errgroup.Group, ops, sz *atomic.Uint64) {
	// it could be via config, but i am kinda lazy
	ignoreDirs := map[string]bool{
		".git":         true,
		".venv":        true,
		"venv":         true,
		"node_modules": true,
		"__pycache__":  true,
		".idea":        true,
		".vscode":      true,
		".cache":       true,
		".cargo":       true,
		".config":      true,
		".docker":      true,
		".local":       true,
		".rustup":      true,
		".themes":      true,
		"target":       true,
		"go":           true,
		"build":        true,
		"dist":         true,
		"vendor":       true,
	}
	g.Go(func() error {
		defer close(ch)
		return filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if ignoreDirs[info.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				return nil
			}
			if info.Size() == 0 {
				return nil
			}
			ops.Add(1)
			sz.Add(uint64(info.Size()))
			ch <- path
			return nil
		})
	})
}

func hashify(ch <-chan string, g *errgroup.Group, m *sync.Map) {
	for path := range ch {
		g.Go(func() error {
			file, err := os.Open(path)
			if err != nil {
				return err
			}

			hasher := sha256.New()
			if _, err := io.Copy(hasher, file); err != nil {
				file.Close()
				return err
			}
			file.Close()

			hash := fmt.Sprintf("%x", hasher.Sum(nil))

			actual, _ := m.LoadOrStore(hash, &[]string{})
			paths := actual.(*[]string)
			*paths = append(*paths, path)

			return nil
		})
	}
}
