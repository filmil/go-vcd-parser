# Value Change Dump (VCD) File parser

[![Test status](https://github.com/filmil/go-vcd-parser/workflows/Test/badge.svg)](https://github.com/filmil/go-vcd-parser/workflows/Test/badge.svg)

This is a parser for the Value Change Dump files, a.k.a VCD file format. The
file format is defined in the [IEEE Standard 1800-2003][vv]. Specifically, the
format supported at the moment is the 4-value format. Some pragmatic extensions
are supported, such as those produced by the `nvc` VHDL simulator.

The correct behavior of the parser is guarded by a suite of tests. Tests
include:
- Unit tests for specific VCD stanzas
- Unit tests for intersting VCD snippets encountered in the wild.
- Integration tests that parse realistic VCD files that were sampled from
  actual uses.

## Why?

- I wanted one written in go (compiled, static, well-tested). Most open source
  alternatives I could find are written in Python, Perl and C++ (see the
  References section below).
- I needed a confirmation that the code can parse realistic VCD files. Hence,
  the test coverage.

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

- The parser is not streaming. It produces an in-memory representation of the
  VCD file before it is able to write a parsed representation out. As VCD files
  can get extraordinarily large, you may find that some realistic large files
  can not be parsed with success.

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


[bb]: https://github.com/filmil/go-vcd-parser/issues
[ii]: https://hdlfactory.com/note/2024/08/24/bazel-installation-via-the-bazelisk-method/
[vv]: https://ieeexplore.ieee.org/document/10458102

