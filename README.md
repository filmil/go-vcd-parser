# Value Change Dump (VCD) File parser

[![Test](https://github.com/filmil/go-vcd-parser/actions/workflows/test.yml/badge.svg)](https://github.com/filmil/go-vcd-parser/actions/workflows/test.yml)
[![Release](https://github.com/filmil/go-vcd-parser/actions/workflows/release.yml/badge.svg)](https://github.com/filmil/go-vcd-parser/actions/workflows/release.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/filmil/go-vcd-parser.svg)](https://pkg.go.dev/github.com/filmil/go-vcd-parser)
[![Go version](https://img.shields.io/github/go-mod/go-version/filmil/go-vcd-parser)](go.mod)
[![Latest release](https://img.shields.io/github/v/release/filmil/go-vcd-parser?include_prereleases)](https://github.com/filmil/go-vcd-parser/releases)

This is a parser for the Value Change Dump files, a.k.a VCD file format. The
file format is defined in the [IEEE Standard 1800-2003][vv]. Specifically, the
format supported at the moment is the 4-value format. Some pragmatic extensions
are supported, such as those produced by the `nvc` VHDL simulator.

It also reads **FST** (Fast Signal Trace), the compressed binary format written
by GTKWave, Verilator, Icarus Verilog and `nvc`. Both formats produce the same
events, so both convert to the same JSON and the same SQLite database. See
[FST input](#fst-input).

The correct behavior of the parser is guarded by a suite of tests. Tests
include:
- Unit tests for specific VCD stanzas
- Unit tests for interesting VCD snippets encountered in the wild.
- Integration tests that parse realistic VCD files that were sampled from
  actual uses.

## Binaries

Each [release][rr] ships prebuilt static binaries for two programs, four
platforms each:

| Program | What it does |
|---|---|
| `vcdcvt` | Parses a VCD or FST dump and converts it to JSON or a SQLite signals database. |
| `sqlite2drawtiming` | Reads a SQLite signals database and writes [drawtiming](https://drawtiming.sourceforge.net/) input for selected signals to stdout. |

The platforms are `linux-amd64`, `linux-arm64`, `darwin-amd64` and
`darwin-arm64`. Download the file for your platform, make it executable, and
put it on your `PATH`:

```sh
curl -LO https://github.com/filmil/go-vcd-parser/releases/latest/download/vcdcvt-linux-amd64
chmod +x vcdcvt-linux-amd64
sudo mv vcdcvt-linux-amd64 /usr/local/bin/vcdcvt
```

`SHA256SUMS` in each release covers every file. On macOS, the first run may
need Gatekeeper approval: `xattr -d com.apple.quarantine <file>`.

Typical use:

```sh
# VCD to a SQLite signals database.
vcdcvt -in dump.vcd -format sqlite -out signals.db

# The same, from an FST dump. The .fst name is what selects the reader.
vcdcvt -in dump.fst -format sqlite -out signals.db

# Selected signals from that database, as drawtiming input.
sqlite2drawtiming -in signals.db -signal clk -signal reset > timing.dt
```

### Worked example

This diagram comes from `vcd/files/samples/tb.vcd`, a UART testbench dump
in this repository, through the two tools and
[drawtiming](https://drawtiming.sourceforge.net/):

```sh
vcdcvt -in vcd/files/samples/tb.vcd -format sqlite -out tb.db

# `name=>alias` renames a signal for the output. Aliases keep the labels
# short, and drawtiming does not accept the slashes of the full paths.
# The timescale of this dump is 1 fs; -ndots 2500000 draws one time dot
# per 2.5 ns, and the window covers the first 40 ns.
sqlite2drawtiming -in tb.db -min-time 0 -max-time 40000000 -ndots 2500000 \
    -signal '//wb_uart_tb/clk=>clk' \
    -signal '//wb_uart_tb/reset=>reset' \
    -signal '//wb_uart_tb/clkgen/reset_n=>reset_n' \
    > timing.dt

drawtiming -o timing.png --cell-width 48 --cell-height 44 timing.dt
```

![Timing diagram of clk, reset and reset_n from the UART testbench dump](docs/timing-example.png)

The clock runs from the start; at 10 ns `reset` deasserts and `reset_n`
rises with it.

Releases are cut on the fifth of each month when `fix:` or `feat:` commits
landed since the last tag, and on demand from the [Release workflow][ww].
The version number follows [semantic versioning][sv], computed from
[conventional commits][cc]. The binaries are cross-compiled with Bazel and a
[zig-based hermetic C toolchain][hh]; the build takes nothing from the host,
so a release is reproducible from its tag.

[rr]: https://github.com/filmil/go-vcd-parser/releases
[ww]: https://github.com/filmil/go-vcd-parser/actions/workflows/release.yml
[sv]: https://semver.org/
[cc]: https://www.conventionalcommits.org/en/v1.0.0/
[hh]: https://github.com/uber/hermetic_cc_toolchain

## FST input

`vcdcvt` reads an FST dump wherever it reads a VCD one:

```sh
vcdcvt -in dump.fst -format sqlite -out signals.db
vcdcvt -in dump.fst -format json   -out signals.json
```

The reader is chosen from the file name: a `.fst` extension reads FST and
anything else reads VCD. `-in-format` overrides that for a dump named
otherwise:

```sh
vcdcvt -in dump.bin -in-format fst -format sqlite -out signals.db
```

The two readers report the same events, through the same `vcd.Handler`
interface, so everything downstream is shared: the same database schema, the
same JSON, and the same `sqlite2drawtiming` output. A `vcd.Handler` written
for VCD consumes an FST dump with no change:

```go
err := fst.Parse("dump.fst", &c)   // instead of vcd.Parse("dump.vcd", f, &c)
```

Two differences follow from the format rather than from this package:

* **FST needs a file, not a stream.** Its value change blocks are reached
  through a table at the end of the file, so `fst.Parse` takes a path and
  seeks in it, where `vcd.Parse` takes an `io.Reader`. Memory use still does
  not grow with the length of the simulation.
* **Order within one timestamp is not the writer's order.** Timestamps arrive
  in increasing order, but the changes at a single timestamp arrive in the
  order the block holds them. They are simultaneous, so nothing is lost, but
  code that compares two dumps has to treat one timestamp's changes as a set.

A malformed time table is reported as an error rather than converted.

FST does not flag whether a value change block's time table is compressed.
libfst writes the compressed bytes only when they are shorter than the raw
ones, and the reader takes "the stored length differs from the uncompressed
length" to mean compressed. A writer whose compressed table came out exactly
as long as the raw one produces a file that reads as raw varints, and the
deltas decoded out of the deflate stream are plausible ascending numbers. The
dump then converts without complaint and every timestamp in it is wrong.

`vcdcvt` checks each time table before reading the dump: the declared number
of entries has to be readable from the declared number of bytes, using all of
them. It reports the block offset and stops, and does not leave a partly
written output file behind.

The reading is done by [libfst][lf], the MIT-licensed C library split out of
GTKWave, vendored under `third_party/libfst`. That directory's
`LIBFST_VERSION` names the revision and its `LICENSE` holds the license,
which also covers the LZ4 and FastLZ sources libfst bundles. It is the one
part of this repository that is not Go.

libfst is built as a `cc_library`, and zlib, the one library it needs and
does not bundle, comes from the `zlib` module, so the cross-compiled release
binaries stay hermetic.

This is the one package that Bazel has to build. cgo compiles the C of a
package from that package's own directory, and libfst is not in it, so
`go build ./...` does not build `fst`. The rest of the repository still
builds either way.

Regenerate the test dump at `cvt/testdata/small.fst` with:

```sh
bazel run //bin/fstgen -- $PWD/cvt/testdata/small.fst
```

The regenerated file differs from the old one byte for byte even when nothing
else changed, because libfst stamps the creation date into the header. The
signals and the value changes are the same.

## Streaming

`vcdcvt` reads a dump as a stream. Memory use is proportional to the header --
the number of signals -- and not to the length of the simulation, for both
output formats. Converting the 11 MB `vcd/files/samples/tb.vcd` in this
repository:

| | peak RSS | wall time |
|---|---|---|
| `--format=sqlite`, before | 1.11 GB | 25.9 s |
| `--format=sqlite`, now | 19 MB | 2.3 s |
| `--format=json`, before | 2.11 GB | 11.7 s |
| `--format=json`, now | 17 MB | 2.7 s |

The output is byte-for-byte what it was: the same SQLite rows in the same
order, and the same JSON.

Two separate things got it there, and the benchmarks in `vcd/files` and `db`
keep both honest. `bazel test //db:db_test //vcd/files:files_test
--test_output=all --test_arg=-test.bench=. --test_arg=-test.run=XXX` runs
them.

**Parsing** went from 9.5 s to 0.095 s -- `BenchmarkParseGrammar` against
`BenchmarkParse` -- because the grammar-driven parser held the file, its
whole token stream and the parse tree at once.

**Writing the database** was then the other 90%, and takes four changes,
each measured by a benchmark in `db/bench_test.go`:

| | rows/s |
|---|---|
| `InsertExecPerRow` -- a statement prepared per row | ~64k |
| `InsertPreparedPerRow` -- prepared once | ~120k |
| `InsertBatchedIndexed` -- 128 rows per statement | ~120k |
| `InsertBulk` -- indexes built afterwards, no journal | ~490k |

The middle pair is worth a look: while the indexes exist, batching buys
nothing, because maintaining them row by row costs more than the driver
does. Batching only pays once the indexes are deferred.

`db.OpenBulk` is what applies this, and `db.FinishBulk` builds the indexes
at the end, leaving a database identical to one `db.OpenDB` would have
made. It turns off the rollback journal and fsync for the load: a
conversion writes a file that did not exist before and is worthless if the
run fails, so there is nothing for them to protect. `db.OpenDB` is
unchanged for every other use.

To consume a dump directly, implement `vcd.Handler` and call `vcd.Parse`:

```go
type counter struct{ vcd.NopHandler; n int }

func (c *counter) ValueChange(k vcd.ValueKind, value, idcode []byte) error {
	c.n++
	return nil
}

var c counter
err := vcd.Parse("dump.vcd", f, &c)
```

Embedding `vcd.NopHandler` means only the events you care about need a method.
The `[]byte` arguments alias the reader's buffer and are valid only for the
duration of the call; wrap your handler in `vcd.CopyHandler` if you need to
keep them.

`vcd.ParseFile` still returns a whole `vcd.File` for callers that want the
parse tree. It avoids holding the input text and its token stream, but the
tree itself grows with the dump, so prefer `vcd.Parse` for very large files.

## Why?

- **I wanted one written in go** (compiled, static, well-tested). Most open source
  alternatives I could find are written in Python, Perl and C++ (see the
  References section below).
- **I wanted one that is *tested***. Most alternatives contain no tests at all. Some
  that I have actually tried would just throw an exception when faced with a
  realistic VCD file.
- **I needed a confirmation that the code can parse realistic VCD files.** Hence,
  the test coverage. And samples of realistic VCD files used for testing.

## Go documentation

API documentation for every package is on [pkg.go.dev][pd]:

| Package | What it holds |
|---|---|
| [`vcd`](https://pkg.go.dev/github.com/filmil/go-vcd-parser/vcd) | The VCD lexer and parser. `vcd.Parse` streams a file to a `vcd.Handler`; `vcd.ParseFile` produces a `vcd.File`. |
| [`cvt`](https://pkg.go.dev/github.com/filmil/go-vcd-parser/cvt) | Conversions of the parsed representation. |
| [`fst`](https://pkg.go.dev/github.com/filmil/go-vcd-parser/fst) | The FST reader. `fst.Parse` streams a dump to the same `vcd.Handler`. |
| [`db`](https://pkg.go.dev/github.com/filmil/go-vcd-parser/db) | The SQLite signal database: schema, writers, readers. |
| [`dbq`](https://pkg.go.dev/github.com/filmil/go-vcd-parser/dbq) | The query engine over a signal database: transition lookups, values at a timestamp, timing assertions for tests. |
| [`dbt`](https://pkg.go.dev/github.com/filmil/go-vcd-parser/dbt) | Test helpers that build small signal databases in memory. |

[pd]: https://pkg.go.dev/github.com/filmil/go-vcd-parser

## Prerequisites

* Install `bazel` using the [bazelisk method][ii].

  It should be possible to use the vanilla go environment as well.

## Test

From the root directory, run:

```
bazel test //...
```

This should always pass. [Report a bug][bb] if not.


If you have `go` installed, you can also run:

```
go test ./...
```

While this should pass, I will not necessarily spend time to make it work
with the go toolkit.

## Limitations

- **VCD format is not ideal.** It cannot describe structured types as defined.
  Some pragmatic extensions make this better, but if you are exporting into the
  VCD format from say Vivado's `xsim`, it will produce a significantly subpar
  output versus Vivado's native (binary and undocumented) WDB format.

## Troubleshooting

If you find a problem VCD file, file a bug report and consider sending the file.
Minimal examples are preferred.

# References

Prior art:

- https://github.com/ben-marshall/verilog-vcd-parser
- https://wohali.github.io/vcd_parsealyze
- https://github.com/kmurray/libvcdparse
- https://pypi.org/project/pyDigitalWaveTools
- https://metacpan.org/pod/Verilog::VCD
- https://pyvcd.readthedocs.io/en/latest/vcd.reader.html
- https://pypi.org/project/vcdvcd/
- https://github.com/gtkwave/gtkwave/blob/0a800de96255f7fb11beadb6729fdf670da76ecb/src/vcd_saver.c#L123
- https://github.com/nickg/nvc/blob/8696f99160eba01c1beb6e243506af57ba9893ca/src/rt/wave.h#L28
- https://github.com/kevinmehall/rust-vcd


[bb]: https://github.com/filmil/go-vcd-parser/issues
[ii]: https://hdlfactory.com/note/2024/08/24/bazel-installation-via-the-bazelisk-method/
[vv]: https://ieeexplore.ieee.org/document/10458102
[lf]: https://github.com/gtkwave/libfst

