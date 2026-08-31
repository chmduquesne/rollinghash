# Benchmarks

Throughput (MiB/s) of every public interface, across every hash.
`benchstat` summary over 15 runs. Regenerate with `./benchmark.sh [count] [benchtime]`.

```
go test -bench='BenchmarkRolling64B|BenchmarkChunker$|BenchmarkChunkWriter$|BenchmarkBatchRoller$|BenchmarkBatchWriter$' -run=^$ -benchtime=1s -count=15 ./... | benchstat -format csv -
```

| Benchmark | Throughput |
|---|---|
| adler32/Rolling64B | 164.0 MiB/s ± 0% |
| BatchRoller/adler32 | 273.5 MiB/s ± 1% |
| BatchRoller/bozo32 | 909.6 MiB/s ± 1% |
| BatchRoller/bozo64 | 911.0 MiB/s ± 1% |
| BatchRoller/buzhash32 | 1003.6 MiB/s ± 1% |
| BatchRoller/buzhash64 | 1032.4 MiB/s ± 2% |
| BatchRoller/gearhash64 | 1011.4 MiB/s ± 1% |
| BatchRoller/rabinkarp64 | 556.7 MiB/s ± 1% |
| BatchWriter/adler32 | 274.3 MiB/s ± 1% |
| BatchWriter/bozo32 | 926.4 MiB/s ± 0% |
| BatchWriter/bozo64 | 928.8 MiB/s ± 0% |
| BatchWriter/buzhash32 | 996.4 MiB/s ± 1% |
| BatchWriter/buzhash64 | 1028.3 MiB/s ± 2% |
| BatchWriter/gearhash64 | 1042.6 MiB/s ± 0% |
| BatchWriter/rabinkarp64 | 586.0 MiB/s ± 0% |
| bozo32/Rolling64B | 586.2 MiB/s ± 0% |
| bozo64/Rolling64B | 576.7 MiB/s ± 0% |
| buzhash32/Rolling64B | 585.0 MiB/s ± 0% |
| buzhash64/Rolling64B | 586.8 MiB/s ± 0% |
| Chunker/adler32/fused | 272.6 MiB/s ± 0% |
| Chunker/bozo32/fused | 796.2 MiB/s ± 1% |
| Chunker/bozo64/fused | 785.2 MiB/s ± 0% |
| Chunker/buzhash32/fused | 1003.3 MiB/s ± 0% |
| Chunker/buzhash64/fused | 1025.1 MiB/s ± 0% |
| Chunker/gearhash64/fused | 1014.9 MiB/s ± 0% |
| Chunker/rabinkarp64/fused | 535.9 MiB/s ± 0% |
| ChunkWriter/adler32/fused | 272.7 MiB/s ± 0% |
| ChunkWriter/bozo32/fused | 797.8 MiB/s ± 0% |
| ChunkWriter/bozo64/fused | 788.2 MiB/s ± 0% |
| ChunkWriter/buzhash32/fused | 998.9 MiB/s ± 0% |
| ChunkWriter/buzhash64/fused | 1025.2 MiB/s ± 0% |
| ChunkWriter/gearhash64/fused | 1019.0 MiB/s ± 0% |
| ChunkWriter/rabinkarp64/fused | 534.0 MiB/s ± 0% |
| gearhash64/Rolling64B | 586.9 MiB/s ± 0% |
| rabinkarp64/Rolling64B | 343.0 MiB/s ± 0% |
