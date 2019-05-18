workspace(name = "io_k8s_repo_infra")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "492c3ac68ed9dcf527a07e6a1b2dcbf199c6bf8b35517951467ac32e421c06c1",
    url = "https://github.com/bazelbuild/rules_go/releases/download/0.17.0/rules_go-0.17.0.tar.gz",
)

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")

go_rules_dependencies()

go_register_toolchains()

http_archive(
    name = "io_bazel",
    sha256 = "6860a226c8123770b122189636fb0c156c6e5c9027b5b245ac3b2315b7b55641",
    url = "https://github.com/bazelbuild/bazel/releases/download/0.22.0/bazel-0.22.0-dist.zip",
)
