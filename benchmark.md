# Benchmarks

Throughput (MiB/s) of every public interface, across every hash.
`benchstat` summary over 15 runs. Regenerate with `./benchmark.sh [count] [benchtime]`.

```
go test -bench='BenchmarkRolling64B|BenchmarkChunker$|BenchmarkChunkWriter$|BenchmarkBatchRoller$|BenchmarkBatchWriter$' -run=^$ -benchtime=1s -count=15 ./... | benchstat -format csv -
```

| Benchmark | Throughput |
|---|---|
| adler32/Rolling64B | 241.7 MiB/s ± 1% |
| BatchRoller/adler32 | 406.4 MiB/s ± 1% |
| BatchRoller/bozo32 | 1267.6 MiB/s ± 3% |
| BatchRoller/bozo64 | 1227.3 MiB/s ± 1% |
| BatchRoller/buzhash32 | 1339.2 MiB/s ± 1% |
| BatchRoller/buzhash64 | 1451.8 MiB/s ± 1% |
| BatchRoller/gearhash64 | 1378.0 MiB/s ± 3% |
| BatchRoller/rabinkarp64 | 829.7 MiB/s ± 2% |
| BatchWriter/adler32 | 400.1 MiB/s ± 1% |
| BatchWriter/bozo32 | 1293.5 MiB/s ± 1% |
| BatchWriter/bozo64 | 1292.7 MiB/s ± 1% |
| BatchWriter/buzhash32 | 1330.7 MiB/s ± 1% |
| BatchWriter/buzhash64 | 1442.6 MiB/s ± 1% |
| BatchWriter/gearhash64 | 1429.5 MiB/s ± 2% |
| BatchWriter/rabinkarp64 | 830.8 MiB/s ± 1% |
| bozo32/Rolling64B | 830.7 MiB/s ± 0% |
| bozo64/Rolling64B | 817.2 MiB/s ± 0% |
| buzhash32/Rolling64B | 789.9 MiB/s ± 4% |
| buzhash64/Rolling64B | 814.5 MiB/s ± 2% |
| Chunker/adler32/fused | 375.6 MiB/s ± 1% |
| Chunker/bozo32/fused | 1127.8 MiB/s ± 0% |
| Chunker/bozo64/fused | 1131.0 MiB/s ± 1% |
| Chunker/buzhash32/fused | 1409.2 MiB/s ± 1% |
| Chunker/buzhash64/fused | 1457.4 MiB/s ± 1% |
| Chunker/gearhash64/fused | 1453.4 MiB/s ± 1% |
| Chunker/rabinkarp64/fused | 762.0 MiB/s ± 1% |
| ChunkWriter/adler32/fused | 384.2 MiB/s ± 0% |
| ChunkWriter/bozo32/fused | 1127.8 MiB/s ± 0% |
| ChunkWriter/bozo64/fused | 1126.4 MiB/s ± 0% |
| ChunkWriter/buzhash32/fused | 1424.1 MiB/s ± 0% |
| ChunkWriter/buzhash64/fused | 1453.3 MiB/s ± 0% |
| ChunkWriter/gearhash64/fused | 1447.6 MiB/s ± 0% |
| ChunkWriter/rabinkarp64/fused | 748.1 MiB/s ± 1% |
| gearhash64/Rolling64B | 811.0 MiB/s ± 1% |
| rabinkarp64/Rolling64B | 488.7 MiB/s ± 0% |
