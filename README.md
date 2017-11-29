[![Build Status](https://travis-ci.org/chmduquesne/rollinghash.svg?branch=master)](https://travis-ci.org/chmduquesne/rollinghash)
[![Coverage Status](https://coveralls.io/repos/github/chmduquesne/rollinghash/badge.svg?branch=master)](https://coveralls.io/github/chmduquesne/rollinghash?branch=master)
[![GoDoc Reference](http://godoc.org/github.com/chmduquesne/rollinghash?status.svg)](https://godoc.org/github.com/chmduquesne/rollinghash)

rolling checksums
=================

Philosophy
----------

This package contains several various rolling checksums for you to play
with crazy ideas. The API design philosophy was to stick as closely as
possible to the interface provided by the builtin hash package, while
providing simultaneously the highest speed and simplicity.

The `Hash32` and `Hash64` interfaces both implement the builtin `Hash32`
and `Hash64`, so that you can use them as drop in replacements. On top of
the builtin methods, these interfaces also implement `Roller`, which
consists in the single method `Roll(b byte)`, designed to update the
rolling checksum with the byte entering the rolling window.

Usage
-----

The rolling window MUST be initialized by calling `Write` first (which
saves a copy). Several calls to `Write` will overwrite this window every
time. The byte leaving the rolling window is inferred from the internal
copy of the rolling window, which is updated with every call to `Roll`.

Be aware that `Roll` never fails: whenever no Rolling window has been
initialized, the implementation assumes a rolling window of 1 byte,
initialized with the null byte. As a consequence, rolling an empty window
returns an incorrect answer. In previous versions of this library, `Roll`
would return an error for this particular case. This change was made in
the interest of speed, so that we don't have to check whether a window
exists for every call, sparing an operation that is useless when the hash
is correctly used, in a function likely to be called millions of times per
second.

License
-------

This code is delivered to you under the terms of the MIT public license,
except the RabinKarp64 subpackage, which has been adapted from
[restic](https://github.com/restic/chunker) (BSD 2-clause "Simplified").

Notable users
-------------

* [syncthing](https://syncthing.net/), a decentralized synchronisation
  solution
* [muscato](https://github.com/kshedden/muscato), a genome analysis tool

If you are using this in production or for research, let me know and I
will happily put a link here!
