workspace(name = "io_kubernetes_build")

git_repository(
    name = "io_bazel_rules_go",
    commit = "f083e0f8bfc4ab3cf58a027ecd4740e1af632212",
    remote = "https://github.com/bazelbuild/rules_go.git",
)

load("@io_bazel_rules_go//go:def.bzl", "go_repositories")

go_repositories()
