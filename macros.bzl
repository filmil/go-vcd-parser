# Generate analysis output from a VCD file.

def vcd_index(name, vcd_target):
    """Creates a VCD file index from the given target, which must have exactly
    one VCD output.
    """
    _sqlite_name = "{}.signals.sqlite".format(name)
    _signals_name = "{}.signals.csv".format(name)
    _vcd_name = "$(location {})".format(vcd_target)
    _command = "$(location {})".format("//bin/vcdcvt")
    native.genrule(
        name = "{}_index".format(name),
        srcs = [vcd_target],
        outs = [_sqlite_name, _signals_name],
        message = "Indexing VCD {}".format(vcd_target),
        cmd = """
        {command} --format=sqlite --logtostderr \
                --in={input} \
                --out=$(location {sqlite}) \
                --signals=$(location {signals})
        """.format(
            command = _command,
            input = _vcd_name,
            sqlite = _sqlite_name,
            signals = _signals_name,
        ),
        tools = [
            vcd_target,
            Label("//bin/vcdcvt"),
        ]
    )
    native.filegroup(
        name = "{}_signals",
        srcs = [ _signals_name ],
    )
    native.filegroup(
        name = name,
        srcs = [ _sqlite_name ],
    )
