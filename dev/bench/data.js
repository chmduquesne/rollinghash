window.BENCHMARK_DATA = {
  "lastUpdate": 1757679647346,
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
      },
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
          "id": "32eb5cee88ee5cf34496d7af309a47a84570dcc9",
          "message": "Update link to benchmark charts",
          "timestamp": "2025-09-11T23:27:27+02:00",
          "tree_id": "8f4bff0a0888f791be292c2984ac282a7f7f2650",
          "url": "https://github.com/chmduquesne/rollinghash/commit/32eb5cee88ee5cf34496d7af309a47a84570dcc9"
        },
        "date": 1757626118200,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkAdler32Rolling64B",
            "value": 6.947,
            "unit": "ns/op\t147409.34 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "172821399 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - ns/op",
            "value": 6.947,
            "unit": "ns/op",
            "extra": "172821399 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - MB/s",
            "value": 147409.34,
            "unit": "MB/s",
            "extra": "172821399 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "172821399 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "172821399 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom",
            "value": 9.818,
            "unit": "ns/op\t104293.37 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "123724783 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - ns/op",
            "value": 9.818,
            "unit": "ns/op",
            "extra": "123724783 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - MB/s",
            "value": 104293.37,
            "unit": "MB/s",
            "extra": "123724783 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "123724783 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "123724783 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B",
            "value": 1.82,
            "unit": "ns/op\t562680.91 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "661551403 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - ns/op",
            "value": 1.82,
            "unit": "ns/op",
            "extra": "661551403 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - MB/s",
            "value": 562680.91,
            "unit": "MB/s",
            "extra": "661551403 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "661551403 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "661551403 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom",
            "value": 6.599,
            "unit": "ns/op\t155171.85 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "181452430 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - ns/op",
            "value": 6.599,
            "unit": "ns/op",
            "extra": "181452430 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - MB/s",
            "value": 155171.85,
            "unit": "MB/s",
            "extra": "181452430 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "181452430 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "181452430 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B",
            "value": 2.584,
            "unit": "ns/op\t396244.94 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "460877282 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - ns/op",
            "value": 2.584,
            "unit": "ns/op",
            "extra": "460877282 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - MB/s",
            "value": 396244.94,
            "unit": "MB/s",
            "extra": "460877282 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "460877282 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "460877282 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom",
            "value": 7.558,
            "unit": "ns/op\t135491.26 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "158881752 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - ns/op",
            "value": 7.558,
            "unit": "ns/op",
            "extra": "158881752 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - MB/s",
            "value": 135491.26,
            "unit": "MB/s",
            "extra": "158881752 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "158881752 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "158881752 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B",
            "value": 2.451,
            "unit": "ns/op\t417772.60 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "484288117 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - ns/op",
            "value": 2.451,
            "unit": "ns/op",
            "extra": "484288117 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - MB/s",
            "value": 417772.6,
            "unit": "MB/s",
            "extra": "484288117 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "484288117 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "484288117 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom",
            "value": 7.944,
            "unit": "ns/op\t128898.63 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "153169798 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - ns/op",
            "value": 7.944,
            "unit": "ns/op",
            "extra": "153169798 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - MB/s",
            "value": 128898.63,
            "unit": "MB/s",
            "extra": "153169798 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "153169798 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "153169798 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolDivMod",
            "value": 4.358,
            "unit": "ns/op",
            "extra": "275221005 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolDiv",
            "value": 4.357,
            "unit": "ns/op",
            "extra": "273304813 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolMod",
            "value": 4.383,
            "unit": "ns/op",
            "extra": "275107906 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolDeg",
            "value": 0.3111,
            "unit": "ns/op",
            "extra": "1000000000 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64RandomPolynomial",
            "value": 83859517,
            "unit": "ns/op",
            "extra": "13 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolIrreducible",
            "value": 15884558,
            "unit": "ns/op",
            "extra": "75 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B",
            "value": 5.542,
            "unit": "ns/op\t184769.93 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "217074691 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - ns/op",
            "value": 5.542,
            "unit": "ns/op",
            "extra": "217074691 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - MB/s",
            "value": 184769.93,
            "unit": "MB/s",
            "extra": "217074691 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "217074691 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "217074691 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom",
            "value": 8.13,
            "unit": "ns/op\t125953.34 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "145505922 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - ns/op",
            "value": 8.13,
            "unit": "ns/op",
            "extra": "145505922 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - MB/s",
            "value": 125953.34,
            "unit": "MB/s",
            "extra": "145505922 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "145505922 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "145505922 times\n4 procs"
          }
        ]
      },
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
          "id": "3450fce14006e5ddcefd82c3c242326583c03271",
          "message": "Add vulnerability checking",
          "timestamp": "2025-09-12T14:18:18+02:00",
          "tree_id": "ddc5bbdd4ab3ffa5057d70d676f3b9d521e9ea38",
          "url": "https://github.com/chmduquesne/rollinghash/commit/3450fce14006e5ddcefd82c3c242326583c03271"
        },
        "date": 1757679583633,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkAdler32Rolling64B",
            "value": 6.988,
            "unit": "ns/op\t146533.93 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "172052618 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - ns/op",
            "value": 6.988,
            "unit": "ns/op",
            "extra": "172052618 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - MB/s",
            "value": 146533.93,
            "unit": "MB/s",
            "extra": "172052618 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "172052618 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "172052618 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom",
            "value": 9.721,
            "unit": "ns/op\t105342.52 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "123104847 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - ns/op",
            "value": 9.721,
            "unit": "ns/op",
            "extra": "123104847 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - MB/s",
            "value": 105342.52,
            "unit": "MB/s",
            "extra": "123104847 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "123104847 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "123104847 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B",
            "value": 1.821,
            "unit": "ns/op\t562464.44 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "663629114 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - ns/op",
            "value": 1.821,
            "unit": "ns/op",
            "extra": "663629114 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - MB/s",
            "value": 562464.44,
            "unit": "MB/s",
            "extra": "663629114 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "663629114 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "663629114 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom",
            "value": 6.62,
            "unit": "ns/op\t154673.60 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "181884948 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - ns/op",
            "value": 6.62,
            "unit": "ns/op",
            "extra": "181884948 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - MB/s",
            "value": 154673.6,
            "unit": "MB/s",
            "extra": "181884948 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "181884948 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "181884948 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B",
            "value": 2.592,
            "unit": "ns/op\t394995.95 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "459650281 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - ns/op",
            "value": 2.592,
            "unit": "ns/op",
            "extra": "459650281 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - MB/s",
            "value": 394995.95,
            "unit": "MB/s",
            "extra": "459650281 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "459650281 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "459650281 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom",
            "value": 7.537,
            "unit": "ns/op\t135862.87 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "158557972 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - ns/op",
            "value": 7.537,
            "unit": "ns/op",
            "extra": "158557972 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - MB/s",
            "value": 135862.87,
            "unit": "MB/s",
            "extra": "158557972 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "158557972 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "158557972 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B",
            "value": 2.46,
            "unit": "ns/op\t416320.18 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "486208760 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - ns/op",
            "value": 2.46,
            "unit": "ns/op",
            "extra": "486208760 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - MB/s",
            "value": 416320.18,
            "unit": "MB/s",
            "extra": "486208760 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "486208760 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "486208760 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom",
            "value": 7.828,
            "unit": "ns/op\t130820.66 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "153200961 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - ns/op",
            "value": 7.828,
            "unit": "ns/op",
            "extra": "153200961 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - MB/s",
            "value": 130820.66,
            "unit": "MB/s",
            "extra": "153200961 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "153200961 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "153200961 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolDivMod",
            "value": 4.377,
            "unit": "ns/op",
            "extra": "275192740 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolDiv",
            "value": 4.357,
            "unit": "ns/op",
            "extra": "258809427 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolMod",
            "value": 4.36,
            "unit": "ns/op",
            "extra": "274895436 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolDeg",
            "value": 0.3116,
            "unit": "ns/op",
            "extra": "1000000000 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64RandomPolynomial",
            "value": 83883177,
            "unit": "ns/op",
            "extra": "13 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolIrreducible",
            "value": 15973861,
            "unit": "ns/op",
            "extra": "74 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B",
            "value": 5.535,
            "unit": "ns/op\t184999.18 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "216398570 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - ns/op",
            "value": 5.535,
            "unit": "ns/op",
            "extra": "216398570 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - MB/s",
            "value": 184999.18,
            "unit": "MB/s",
            "extra": "216398570 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "216398570 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "216398570 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom",
            "value": 8.323,
            "unit": "ns/op\t123039.75 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "147601629 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - ns/op",
            "value": 8.323,
            "unit": "ns/op",
            "extra": "147601629 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - MB/s",
            "value": 123039.75,
            "unit": "MB/s",
            "extra": "147601629 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "147601629 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "147601629 times\n4 procs"
          }
        ]
      },
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
          "id": "094cfc12307940b6f7505506c5a0ed7e5bf54793",
          "message": "Update changelog",
          "timestamp": "2025-09-12T14:19:37+02:00",
          "tree_id": "ad208916b48834c4fca89e031057478729def9e2",
          "url": "https://github.com/chmduquesne/rollinghash/commit/094cfc12307940b6f7505506c5a0ed7e5bf54793"
        },
        "date": 1757679646336,
        "tool": "go",
        "benches": [
          {
            "name": "BenchmarkAdler32Rolling64B",
            "value": 6.951,
            "unit": "ns/op\t147313.93 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "172507954 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - ns/op",
            "value": 6.951,
            "unit": "ns/op",
            "extra": "172507954 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - MB/s",
            "value": 147313.93,
            "unit": "MB/s",
            "extra": "172507954 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "172507954 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "172507954 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom",
            "value": 9.673,
            "unit": "ns/op\t105864.82 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "122252043 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - ns/op",
            "value": 9.673,
            "unit": "ns/op",
            "extra": "122252043 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - MB/s",
            "value": 105864.82,
            "unit": "MB/s",
            "extra": "122252043 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "122252043 times\n4 procs"
          },
          {
            "name": "BenchmarkAdler32ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "122252043 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B",
            "value": 1.823,
            "unit": "ns/op\t561750.84 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "660420849 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - ns/op",
            "value": 1.823,
            "unit": "ns/op",
            "extra": "660420849 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - MB/s",
            "value": 561750.84,
            "unit": "MB/s",
            "extra": "660420849 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "660420849 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "660420849 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom",
            "value": 6.59,
            "unit": "ns/op\t155377.34 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "182123719 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - ns/op",
            "value": 6.59,
            "unit": "ns/op",
            "extra": "182123719 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - MB/s",
            "value": 155377.34,
            "unit": "MB/s",
            "extra": "182123719 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "182123719 times\n4 procs"
          },
          {
            "name": "BenchmarkBozo32ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "182123719 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B",
            "value": 2.594,
            "unit": "ns/op\t394759.68 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "462033048 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - ns/op",
            "value": 2.594,
            "unit": "ns/op",
            "extra": "462033048 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - MB/s",
            "value": 394759.68,
            "unit": "MB/s",
            "extra": "462033048 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "462033048 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "462033048 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom",
            "value": 7.543,
            "unit": "ns/op\t135749.20 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "158098416 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - ns/op",
            "value": 7.543,
            "unit": "ns/op",
            "extra": "158098416 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - MB/s",
            "value": 135749.2,
            "unit": "MB/s",
            "extra": "158098416 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "158098416 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash32ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "158098416 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B",
            "value": 2.448,
            "unit": "ns/op\t418252.98 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "491548658 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - ns/op",
            "value": 2.448,
            "unit": "ns/op",
            "extra": "491548658 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - MB/s",
            "value": 418252.98,
            "unit": "MB/s",
            "extra": "491548658 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "491548658 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "491548658 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom",
            "value": 7.883,
            "unit": "ns/op\t129892.27 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "152773842 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - ns/op",
            "value": 7.883,
            "unit": "ns/op",
            "extra": "152773842 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - MB/s",
            "value": 129892.27,
            "unit": "MB/s",
            "extra": "152773842 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "152773842 times\n4 procs"
          },
          {
            "name": "BenchmarkBuzhash64ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "152773842 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolDivMod",
            "value": 4.433,
            "unit": "ns/op",
            "extra": "240615122 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolDiv",
            "value": 4.355,
            "unit": "ns/op",
            "extra": "275347370 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolMod",
            "value": 4.357,
            "unit": "ns/op",
            "extra": "275297389 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolDeg",
            "value": 0.3114,
            "unit": "ns/op",
            "extra": "1000000000 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64RandomPolynomial",
            "value": 84298757,
            "unit": "ns/op",
            "extra": "13 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64PolIrreducible",
            "value": 15888511,
            "unit": "ns/op",
            "extra": "74 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B",
            "value": 5.523,
            "unit": "ns/op\t185391.30 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "216752994 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - ns/op",
            "value": 5.523,
            "unit": "ns/op",
            "extra": "216752994 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - MB/s",
            "value": 185391.3,
            "unit": "MB/s",
            "extra": "216752994 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "216752994 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64Rolling64B - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "216752994 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom",
            "value": 8.152,
            "unit": "ns/op\t125616.47 MB/s\t       0 B/op\t       0 allocs/op",
            "extra": "147256659 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - ns/op",
            "value": 8.152,
            "unit": "ns/op",
            "extra": "147256659 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - MB/s",
            "value": 125616.47,
            "unit": "MB/s",
            "extra": "147256659 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - B/op",
            "value": 0,
            "unit": "B/op",
            "extra": "147256659 times\n4 procs"
          },
          {
            "name": "BenchmarkRabinkarp64ReadUrandom - allocs/op",
            "value": 0,
            "unit": "allocs/op",
            "extra": "147256659 times\n4 procs"
          }
        ]
      }
    ]
  }
}