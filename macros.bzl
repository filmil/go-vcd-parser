load("@io_bazel_rules_go//go:def.bzl", "go_test")


# Generate analysis output from a VCD file.
def vcd_index(name, vcd_target):
    """Creates a VCD file index from the given target, which must have exactly
    one VCD output.
    """
    _label = Label("//bin/vcdcvt")
    _sqlite_name = "{}.signals.sqlite".format(name)
    _signals_name = "{}.signals.csv".format(name)
    _vcd_name = "$(locations {})".format(vcd_target)
    _command = "$(location {})".format(_label)
    native.genrule(
        name = "{}_index".format(name),
        srcs = [vcd_target],
        outs = [_sqlite_name, _signals_name],
        message = "Indexing VCD {}".format(vcd_target),
        cmd = """
        {command} --format=sqlite --logtostderr \
                --out=$(location {sqlite}) \
                --signals=$(location {signals}) \
                --in={input}
        """.format(
            command = _command,
            input = _vcd_name,
            sqlite = _sqlite_name,
            signals = _signals_name,
        ),
        tools = [
            vcd_target,
            _label,
        ]
    )
    native.filegroup(
        name = "{}_signals".format(name),
        srcs = [ _signals_name ],
    )
    native.filegroup(
        name = name,
        srcs = [ _sqlite_name ],
    )


def vcd_go_test(name, vcd_file, args=[], data=[], **kw):
   _args = args + [ "--test-db-name=_main/$(location {})".format(vcd_file)]
   _data = data + [ vcd_file ]
   go_test(
       name = name,
       args = _args,
       data = _data,
       **kw
   )


def index_to_drawtiming(name, sqlite_name, args=[]):
    _tool = Label("//bin/sqlite2drawtiming")
    _outfile = "{}.drawtiming.t"
    _name = "{}_gen".format(name)
    native.genrule(
        name = _name,
        srcs = [sqlite_name],
        outs = [_outfile],
        tools = [
            _tool,
        ],
        cmd = """
            {command} {args} --in={sqlite} > {outfile}
        """.format(
            command="$(location {})".format(_tool),
            args=" ".join(args),
            outfile=_outfile,
            sqlite=sqlite_name,
        ),
    )
    native.filegroup(
        name = name,
        srcs = [
            ":{}".format(_name)
        ]
    )

