# Deduplication

A fast duplicate file finder written in Go that identifies duplicate files based on content.

## What it does

Recursively scans directories, hashes file contents, and reports which files are duplicates. Built to be fast and efficient using Go's concurrency primitives.

## Performance

Benchmarked against fdupes on a home directory with ~390k files and 21 GiB of data:
```bash
> hyperfine './deduplication ~' "fdupes -r ~" --warmup=1
Benchmark 1: ./deduplication ~
  Time (mean ± σ):     39.049 s ±  2.016 s    [User: 52.885 s, System: 17.144 s]
  Range (min … max):   35.483 s … 42.315 s    10 runs

Benchmark 2: fdupes -r ~
  Time (mean ± σ):     152.610 s ±  2.841 s    [User: 79.181 s, System: 17.412 s]
  Range (min … max):   148.829 s … 158.604 s    10 runs

Summary
  ./deduplication ~ ran
    3.91 ± 0.21 times faster than fdupes -r ~
```

