window.BENCHMARK_DATA = {
  "lastUpdate": 1757625925465,
  "repoUrl": "https://github.com/chmduquesne/rollinghash",
  "entries": {
    "Go Benchmark": [
      {
        "commit": {
          "author": {
            "email": "chmd@chmd.fr",
            "name": "Christophe-Marie Duquesne",
            "username": "chmduquesne"
          },
          "committer": {
            "email": "chmd@chmd.fr",
            "name": "Christophe-Marie Duquesne",
            "username": "chmduquesne"
          },
          "distinct": true,
          "id": "91991f10b9e1f6d93010eb3128d1e79f54e3291f",
          "message": "Change the alert threshold",
          "timestamp": "2025-09-11T23:24:01+02:00",
          "tree_id": "a031bae555056daf6612196a571affdef8187c4b",
          "url": "https://github.com/chmduquesne/rollinghash/commit/91991f10b9e1f6d93010eb3128d1e79f54e3291f"
        },
        "date": 1757625925129,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkAdler32Rolling64B",
            "value": 6.948,
            "unit": "ns/op\t147388.53 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "172268924 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - ns/op",
            "value": 6.948,
            "unit": "ns/op",
            "extra": "172268924 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - MB/s",
            "value": 147388.53,
            "unit": "MB/s",
            "extra": "172268924 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "172268924 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "172268924 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom",
            "value": 9.734,
            "unit": "ns/op\t105195.40 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "123461948 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - ns/op",
            "value": 9.734,
            "unit": "ns/op",
            "extra": "123461948 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - MB/s",
            "value": 105195.4,
            "unit": "MB/s",
            "extra": "123461948 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "123461948 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "123461948 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B",
            "value": 1.824,
            "unit": "ns/op\t561452.27 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "661451820 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - ns/op",
            "value": 1.824,
            "unit": "ns/op",
            "extra": "661451820 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - MB/s",
            "value": 561452.27,
            "unit": "MB/s",
            "extra": "661451820 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "661451820 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "661451820 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom",
            "value": 6.594,
            "unit": "ns/op\t155295.46 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "180443233 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - ns/op",
            "value": 6.594,
            "unit": "ns/op",
            "extra": "180443233 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - MB/s",
            "value": 155295.46,
            "unit": "MB/s",
            "extra": "180443233 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "180443233 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "180443233 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B",
            "value": 2.594,
            "unit": "ns/op\t394686.46 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "461703507 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - ns/op",
            "value": 2.594,
            "unit": "ns/op",
            "extra": "461703507 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - MB/s",
            "value": 394686.46,
            "unit": "MB/s",
            "extra": "461703507 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "461703507 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "461703507 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom",
            "value": 7.537,
            "unit": "ns/op\t135863.90 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "159562581 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - ns/op",
            "value": 7.537,
            "unit": "ns/op",
            "extra": "159562581 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - MB/s",
            "value": 135863.9,
            "unit": "MB/s",
            "extra": "159562581 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "159562581 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "159562581 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B",
            "value": 2.456,
            "unit": "ns/op\t416934.22 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "490216147 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - ns/op",
            "value": 2.456,
            "unit": "ns/op",
            "extra": "490216147 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - MB/s",
            "value": 416934.22,
            "unit": "MB/s",
            "extra": "490216147 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "490216147 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "490216147 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom",
            "value": 8.033,
            "unit": "ns/op\t127470.80 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "151856341 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - ns/op",
            "value": 8.033,
            "unit": "ns/op",
            "extra": "151856341 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - MB/s",
            "value": 127470.8,
            "unit": "MB/s",
            "extra": "151856341 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "151856341 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "151856341 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolDivMod",
            "value": 4.36,
            "unit": "ns/op",
            "extra": "275281034 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolDiv",
            "value": 4.357,
            "unit": "ns/op",
            "extra": "275219766 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolMod",
            "value": 4.356,
            "unit": "ns/op",
            "extra": "275019357 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolDeg",
            "value": 0.3122,
            "unit": "ns/op",
            "extra": "1000000000 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64RandomPolynomial",
            "value": 83776421,
            "unit": "ns/op",
            "extra": "13 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolIrreducible",
            "value": 15915703,
            "unit": "ns/op",
            "extra": "74 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B",
            "value": 5.53,
            "unit": "ns/op\t185182.16 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "217030814 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - ns/op",
            "value": 5.53,
            "unit": "ns/op",
            "extra": "217030814 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - MB/s",
            "value": 185182.16,
            "unit": "MB/s",
            "extra": "217030814 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "217030814 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "217030814 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom",
            "value": 8.138,
            "unit": "ns/op\t125824.36 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "147714272 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - ns/op",
            "value": 8.138,
            "unit": "ns/op",
            "extra": "147714272 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - MB/s",
            "value": 125824.36,
            "unit": "MB/s",
            "extra": "147714272 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "147714272 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "147714272 times\n4 procs"
          }
        ]
      }
    ]
  }
}