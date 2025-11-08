package main

import (
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

type safeSlice struct {
	mu    sync.Mutex
	paths []string
}

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

	var ops, sz atomic.Uint64
	var sizeMap sync.Map

	g := new(errgroup.Group)
	g.Go(func() error {
		return walk(root, &sizeMap, &ops, &sz)
	})

	if err := g.Wait(); err != nil {
		return err
	}

	var hashMap sync.Map
	g2 := new(errgroup.Group)
	g2.SetLimit(30)

	sizeMap.Range(func(key, value any) bool {
		ss := value.(*safeSlice)

		ss.mu.Lock()
		paths := make([]string, len(ss.paths))
		copy(paths, ss.paths)
		ss.mu.Unlock()
		if len(paths) <= 1 {
			return true
		}
		for _, p := range paths {
			g2.Go(func() error {
				return hashify(p, &hashMap)
			})
		}
		return true
	})

	if err := g2.Wait(); err != nil {
		return err
	}

	dupcnt := 0
	foundAny := false
	hashMap.Range(func(key, value any) bool {
		ss, ok := value.(*safeSlice)
		if !ok || len(ss.paths) <= 1 {
			return true
		}

		dupcnt++
		if !foundAny {
			fmt.Println("Duplicate files found:")
			foundAny = true
		}
		fmt.Printf("\nHash: %s\n", key)
		for _, p := range ss.paths {
			fmt.Printf("  %s\n", p)
		}
		return true
	})

	if !foundAny {
		fmt.Println("All files are unique")
	}

	mb := sz.Load() / 1024 / 1024
	kb := sz.Load()/1024 - mb*1024
	fmt.Println("Duplicate files:", dupcnt)
	fmt.Printf("File scanned: %d\n", ops.Load())
	fmt.Printf("Total files size sum: %d.%d MB\n", mb, kb)
	fmt.Printf("Time taken: %v\n", time.Since(start))
	return nil
}

func walk(root string, m *sync.Map, ops, sz *atomic.Uint64) error {
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

		actual, _ := m.LoadOrStore(info.Size(), &safeSlice{})
		ss := actual.(*safeSlice)

		ss.mu.Lock()
		ss.paths = append(ss.paths, path)
		ss.mu.Unlock()

		return nil
	})
}

func hashify(path string, m *sync.Map) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}

	hasher := fnv.New64a()
	if _, err := io.Copy(hasher, file); err != nil {
		file.Close()
		return err
	}
	file.Close()

	hash := fmt.Sprintf("%x", hasher.Sum(nil))

	actual, _ := m.LoadOrStore(hash, &safeSlice{})
	ss := actual.(*safeSlice)

	ss.mu.Lock()
	ss.paths = append(ss.paths, path)
	ss.mu.Unlock()

	return nil
}
