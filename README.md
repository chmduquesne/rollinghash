[![Build Status](https://travis-ci.org/chmduquesne/rollinghash.svg?branch=master)](https://travis-ci.org/chmduquesne/rollinghash)
[![Coverage Status](https://coveralls.io/repos/github/chmduquesne/rollinghash/badge.svg?branch=master)](https://coveralls.io/github/chmduquesne/rollinghash?branch=master)
[![GoDoc Reference](http://godoc.org/gopkg.in/chmduquesne/rollinghash.v1?status.svg)](https://godoc.org/gopkg.in/chmduquesne/rollinghash.v2)

This package contains several various rolling checksums for you to play
with crazy ideas. The API design philosophy was to stick as closely as
possible to the interface provided by the builtin hash package, while
providing simultaneously the highest speed and simplicity.

The Hash32 and Hash64 interfaces provided in this package both implement
the builtin Hash32 and Hash64 interfaces, so that you can use them as drop
in replacements for their builtin counterparts (in fact, whenever it
exists, the builtin is used). On top of the builtin methods, these
interfaces also implement the Roller interface, which consists in the
single method Roll(b byte), designed to update the rolling checksum with
the byte entering the rolling window.

The rolling window is assumed to be previously initialized by calling the
Write method provided by the embedded io.Writer, that we hijack to save a
copy. Several calls to Write will overwrite this window every time. The
byte leaving the rolling window is inferred from the internal copy of the
rolling window, which is updated by each call to Roll().

Be aware that Roll() never fails: whenever no Rolling window has been
initialized, the implementation assumes a rolling window of 1 byte,
initialized with the null byte. As a consequence, rolling an empty window
returns an incorrect answer. In previous versions of this library, Roll
would return an error for this particular case. This change was made in
the interest of speed, so that we don't have to check whether a window
exists for every call, sparing an operation that is useless when the hash
is correctly used, in a function likely to be called millions of times per
second.
