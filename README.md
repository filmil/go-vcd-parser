# Value Change Dump (VCD) File parser [![Test status](https://github.com/filmil/go-vcd-parser/workflows/Test/badge.svg)](https://github.com/filmil/go-vcd-parser/workflows/Test/badge.svg)

This is a parser for the Value Change Dump files, a.k.a VCD file format.

## Why?

I could not find one that both exists, and works.

## Prerequisites

* Install `bazel` using the [bazelisk method][ii].

  It should be possible to use the vanilla go environment as well.

[ii]: https://hdlfactory.com/note/2024/08/24/bazel-installation-via-the-bazelisk-method/

## Test

From the root directory, run:

```
bazel test //...
```

This should always pass. Report a bug if not.

If you have `go` installed, you can also run:

```
go test ./...
```

While this should pass, I will not necessarily spend time to make it work
with the go toolkit.

## Troubleshooting

If you find a problem VCD file, file a bug report and consider sending the file.
Minimal examples are preferred.

