load("@bazel_gazelle//:deps.bzl", "go_repository")

def go_dependencies():
    go_repository(
        name = "com_github_alecthomas_assert_v2",
        importpath = "github.com/alecthomas/assert/v2",
        sum = "h1:2Q9r3ki8+JYXvGsDyBXwH3LcJ+WK5D0gc5E8vS6K3D0=",
        version = "v2.11.0",
    )
    go_repository(
        name = "com_github_alecthomas_participle_v2",
        importpath = "github.com/alecthomas/participle/v2",
        sum = "h1:W/H79S8Sat/krZ3el6sQMvMaahJ+XcM9WSI2naI7w2U=",
        version = "v2.1.4",
    )
    go_repository(
        name = "com_github_alecthomas_repr",
        importpath = "github.com/alecthomas/repr",
        sum = "h1:GhI2A8MACjfegCPVq9f1FLvIBS+DrQ2KQBFZP1iFzXc=",
        version = "v0.4.0",
    )
    go_repository(
        name = "com_github_davecgh_go_spew",
        importpath = "github.com/davecgh/go-spew",
        sum = "h1:vj9j/u1bqnvCEfJOwUhtlOARqs3+rkHYY13jYWTU97c=",
        version = "v1.1.1",
    )
    go_repository(
        name = "com_github_dsnet_golib_unitconv",
        importpath = "github.com/dsnet/golib/unitconv",
        sum = "h1:45gXng3Op1vTrnX1PdM9Bla4mEpBFYA5aC8dlqacmwM=",
        version = "v1.0.2",
    )
    go_repository(
        name = "com_github_golang_glog",
        importpath = "github.com/golang/glog",
        sum = "h1:DrW6hGnjIhtvhOIiAKT6Psh/Kd/ldepEa81DKeiRJ5I=",
        version = "v1.2.5",
    )
    go_repository(
        name = "com_github_google_go_cmp",
        importpath = "github.com/google/go-cmp",
        sum = "h1:ofyhxvXcZhMsU5ulbFiLKl/XBFqE1GSq7atu8tAmTRI=",
        version = "v0.6.0",
    )
    go_repository(
        name = "com_github_hexops_gotextdiff",
        importpath = "github.com/hexops/gotextdiff",
        sum = "h1:gitA9+qJrrTCsiCl7+kh75nPqQt1cx4ZkudSTLoUqJM=",
        version = "v1.0.3",
    )
    go_repository(
        name = "com_github_mattn_go_sqlite3",
        importpath = "github.com/mattn/go-sqlite3",
        sum = "h1:JD12Ag3oLy1zQA+BNn74xRgaBbdhbNIDYvQUEuuErjs=",
        version = "v1.14.32",
    )
    go_repository(
        name = "org_golang_x_mod",
        importpath = "golang.org/x/mod",
        sum = "h1:kb+q2PyFnEADO2IEF935ehFUXlWiNjJWtRNgBLSfbxQ=",
        version = "v0.27.0",
    )
    go_repository(
        name = "org_golang_x_sync",
        importpath = "golang.org/x/sync",
        sum = "h1:l60nONMj9l5drqw6jlhIELNv9I0A4OFgRsG9k2oT9Ug=",
        version = "v0.17.0",
    )
    go_repository(
        name = "org_golang_x_text",
        importpath = "golang.org/x/text",
        sum = "h1:1neNs90w9YzJ9BocxfsQNHKuAT4pkghyXc4nhZ6sJvk=",
        version = "v0.29.0",
    )
    go_repository(
        name = "org_golang_x_tools",
        importpath = "golang.org/x/tools",
        sum = "h1:kWS0uv/zsvHEle1LbV5LE8QujrxB3wfQyxHfhOk0Qkg=",
        version = "v0.36.0",
    )
