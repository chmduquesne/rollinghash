# Benchmarks

Throughput (MiB/s) of every public interface, across every hash.
`benchstat` summary over 6 runs. Regenerate with `./benchmark.sh [count] [benchtime]`.

```
go test -bench='BenchmarkRolling64B|BenchmarkChunker$|BenchmarkChunkWriter$|BenchmarkBatchRoller$|BenchmarkBatchWriter$' -run=^$ -benchtime=1s -count=6 ./... | benchstat -format csv -
```

| Benchmark | Throughput |
|---|---|
| adler32/Rolling64B | 164.2 MiB/s ± 0% |
| BatchRoller/adler32 | 273.7 MiB/s ± 1% |
| BatchRoller/bozo32 | 893.9 MiB/s ± 1% |
| BatchRoller/bozo64 | 907.2 MiB/s ± 1% |
| BatchRoller/buzhash32 | 994.6 MiB/s ± 1% |
| BatchRoller/buzhash64 | 996.0 MiB/s ± 2% |
| BatchRoller/gearhash64 | 1021.7 MiB/s ± 2% |
| BatchRoller/rabinkarp64 | 568.3 MiB/s ± 3% |
| BatchWriter/adler32 | 272.5 MiB/s ± 2% |
| BatchWriter/bozo32 | 910.1 MiB/s ± 1% |
| BatchWriter/bozo64 | 921.9 MiB/s ± 1% |
| BatchWriter/buzhash32 | 999.3 MiB/s ± 1% |
| BatchWriter/buzhash64 | 1019.4 MiB/s ± 3% |
| BatchWriter/gearhash64 | 1028.1 MiB/s ± 1% |
| BatchWriter/rabinkarp64 | 567.0 MiB/s ± 1% |
| bozo32/Rolling64B | 586.5 MiB/s ± 0% |
| bozo64/Rolling64B | 575.7 MiB/s ± 1% |
| buzhash32/Rolling64B | 584.5 MiB/s ± 1% |
| buzhash64/Rolling64B | 584.9 MiB/s ± 1% |
| Chunker/adler32/fused | 269.7 MiB/s ± 2% |
| Chunker/bozo32/fused | 788.3 MiB/s ± 2% |
| Chunker/bozo64/fused | 780.1 MiB/s ± 2% |
| Chunker/buzhash32/fused | 993.0 MiB/s ± 2% |
| Chunker/buzhash64/fused | 1017.0 MiB/s ± 1% |
| Chunker/gearhash64/fused | 1018.0 MiB/s ± 1% |
| Chunker/rabinkarp64/fused | 527.2 MiB/s ± 2% |
| ChunkWriter/adler32/fused | 271.8 MiB/s ± 0% |
| ChunkWriter/bozo32/fused | 793.5 MiB/s ± 0% |
| ChunkWriter/bozo64/fused | 789.0 MiB/s ± 1% |
| ChunkWriter/buzhash32/fused | 1002.7 MiB/s ± 1% |
| ChunkWriter/buzhash64/fused | 1018.8 MiB/s ± 1% |
| ChunkWriter/gearhash64/fused | 1018.9 MiB/s ± 0% |
| ChunkWriter/rabinkarp64/fused | 535.4 MiB/s ± 1% |
| gearhash64/Rolling64B | 584.8 MiB/s ± 2% |
| rabinkarp64/Rolling64B | 344.3 MiB/s ± 0% |
