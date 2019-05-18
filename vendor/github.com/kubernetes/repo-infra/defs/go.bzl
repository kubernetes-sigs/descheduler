# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

load("@io_bazel_rules_go//go:def.bzl", "GoLibrary", "GoPath", "go_context", "go_path", "go_rule")

def _compute_genrule_variables(resolved_srcs, resolved_outs, dep_import_paths):
    variables = {
        "SRCS": " ".join([src.path for src in resolved_srcs]),
        "OUTS": " ".join([out.path for out in resolved_outs]),
        "GO_IMPORT_PATHS": " ".join(dep_import_paths),
    }
    if len(resolved_srcs) == 1:
        variables["<"] = resolved_srcs[0].path
    if len(resolved_outs) == 1:
        variables["@"] = resolved_outs[0].path
    return variables

def _go_genrule_impl(ctx):
    go = go_context(ctx)

    transitive_depsets = []
    label_dict = {}
    go_paths = []

    for dep in ctx.attr.srcs:
        transitive_depsets.append(dep.files)
        label_dict[dep.label] = dep.files.to_list()

    dep_import_paths = []
    for dep in ctx.attr.go_deps:
        dep_import_paths.append(dep[GoLibrary].importpath)

    for go_path in ctx.attr.go_paths:
        transitive_depsets.append(go_path.files)
        label_dict[go_path.label] = go_path.files.to_list()

        gp = go_path[GoPath]
        ext = gp.gopath_file.extension
        if ext == "":
            # mode is 'copy' - path is just the gopath
            go_paths.append(gp.gopath_file.path)
        elif ext == "tag":
            # mode is 'link' - path is a tag file in the gopath
            go_paths.append(gp.gopath_file.dirname)
        else:
            fail("Unknown extension on gopath file: '%s'." % ext)

    all_srcs = depset(
        go.sdk.libs + go.sdk.srcs + go.sdk.tools + [go.sdk.go],
        transitive = transitive_depsets,
    )

    cmd = [
        "set -e",
        "export GO_GENRULE_EXECROOT=$$(pwd)",
        # Set GOPATH, GOROOT, and PATH to absolute paths so that commands can chdir without issue
        "export GOPATH=" + ctx.configuration.host_path_separator.join(["$$GO_GENRULE_EXECROOT/" + p for p in go_paths]),
        "export GOROOT=$$GO_GENRULE_EXECROOT/" + go.sdk.root_file.dirname,
        "export PATH=$$GO_GENRULE_EXECROOT/" + go.sdk.root_file.dirname + "/bin:$$PATH",
        ctx.attr.cmd.strip(" \t\n\r"),
    ]
    resolved_inputs, argv, runfiles_manifests = ctx.resolve_command(
        command = "\n".join(cmd),
        attribute = "cmd",
        expand_locations = True,
        make_variables = _compute_genrule_variables(
            all_srcs.to_list(),
            ctx.outputs.outs,
            dep_import_paths,
        ),
        tools = ctx.attr.tools,
        label_dict = label_dict,
    )

    env = {}
    env.update(ctx.configuration.default_shell_env)
    env.update(go.env)
    env["PATH"] = ctx.configuration.host_path_separator.join(["/bin", "/usr/bin"])

    ctx.actions.run_shell(
        inputs = depset(resolved_inputs, transitive = [all_srcs]),
        outputs = ctx.outputs.outs,
        env = env,
        command = argv,
        progress_message = "%s %s" % (ctx.attr.message, ctx),
        mnemonic = "GoGenrule",
    )

# We have codegen procedures that depend on the "go/*" stdlib packages
# and thus depend on executing with a valid GOROOT. _go_genrule handles
# dependencies on the Go toolchain and environment variables; the
# macro go_genrule handles setting up GOPATH dependencies (using go_path).
_go_genrule = go_rule(
    _go_genrule_impl,
    attrs = {
        "srcs": attr.label_list(allow_files = True),
        "tools": attr.label_list(
            cfg = "host",
            allow_files = True,
        ),
        "outs": attr.output_list(mandatory = True),
        "cmd": attr.string(mandatory = True),
        "go_paths": attr.label_list(),
        "go_deps": attr.label_list(providers = [GoLibrary]),
        "importpath": attr.string(),
        "message": attr.string(),
        "executable": attr.bool(default = False),
    },
    output_to_genfiles = True,
)

# Genrule wrapper for tools which need dependencies in a valid GOPATH
# and access to the Go standard library and toolchain.
#
# Go source dependencies specified through the go_deps argument
# are passed to the rules_go go_path rule to build a GOPATH
# for the provided genrule command.
#
# The command can access the generated GOPATH through the GOPATH
# environment variable.
def go_genrule(name, go_deps, **kw):
    go_path_name = "%s~gopath" % name
    go_path(
        name = go_path_name,
        mode = "link",
        visibility = ["//visibility:private"],
        deps = go_deps,
    )
    _go_genrule(
        name = name,
        go_paths = [":" + go_path_name],
        go_deps = go_deps,
        **kw
    )
