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

def _gcs_upload_impl(ctx):
    output_lines = []
    for t in ctx.attr.data:
        label = str(t.label)
        upload_path = ctx.attr.upload_paths.get(label, "")
        for f in t.files:
            output_lines.append("%s\t%s" % (f.short_path, upload_path))

    ctx.file_action(
        output = ctx.outputs.targets,
        content = "\n".join(output_lines),
    )

    ctx.file_action(
        content = "%s --manifest %s --root $PWD -- $@" % (
            ctx.attr.uploader.files_to_run.executable.short_path,
            ctx.outputs.targets.short_path,
        ),
        output = ctx.outputs.executable,
        executable = True,
    )

    return struct(
        runfiles = ctx.runfiles(
            files = ctx.files.data + ctx.files.uploader + [ctx.info_file, ctx.version_file, ctx.outputs.targets],
        ),
    )

# Adds an executable rule to upload the specified artifacts to GCS.
#
# The keys in upload_paths must match the elaborated targets exactly; i.e.,
# one must specify "//foo/bar:bar" and not just "//foo/bar".
#
# Both the upload_paths and the path supplied on the commandline can include
# Python format strings which will be replaced by values from the workspace status,
# e.g. gs://my-bucket-{BUILD_USER}/stash/{STABLE_BUILD_SCM_REVISION}
gcs_upload = rule(
    attrs = {
        "data": attr.label_list(
            mandatory = True,
            allow_files = True,
        ),
        "uploader": attr.label(
            default = Label("//defs:gcs_uploader"),
            allow_files = True,
        ),
        # TODO: combine with 'data' when label_keyed_string_dict is supported in Bazel
        "upload_paths": attr.string_dict(
            allow_empty = True,
        ),
    },
    executable = True,
    outputs = {
        "targets": "%{name}-targets.txt",
    },
    implementation = _gcs_upload_impl,
)

# Computes the md5sum of the provided src file, saving it in a file named 'name'.
def md5sum(name, src, **kwargs):
    native.genrule(
        name = name + "_genmd5sum",
        srcs = [src],
        outs = [name],
        cmd = "command -v md5 >/dev/null && cmd='md5 -q' || cmd=md5sum; $$cmd $< | awk '{print $$1}' >$@",
        message = "Computing md5sum",
        **kwargs
    )

# Computes the sha1sum of the provided src file, saving it in a file named 'name'.
def sha1sum(name, src, **kwargs):
    native.genrule(
        name = name + "_gensha1sum",
        srcs = [src],
        outs = [name],
        cmd = "command -v sha1sum >/dev/null && cmd=sha1sum || cmd='shasum -a1'; $$cmd $< | awk '{print $$1}' >$@",
        message = "Computing sha1sum",
        **kwargs
    )

# Computes the sha512sum of the provided src file, saving it in a file named 'name'.
def sha512sum(name, src, **kwargs):
    native.genrule(
        name = name + "_gensha512sum",
        srcs = [src],
        outs = [name],
        cmd = "command -v sha512sum >/dev/null && cmd=sha512sum || cmd='shasum -a512'; $$cmd $< | awk '{print $$1}' >$@",
        message = "Computing sha512sum",
        **kwargs
    )

# Returns a list of hash target names for the provided srcs.
# Also updates the srcs_basenames_needing_hashes dictionary,
# mapping src name to basename for each target in srcs.
def _hashes_for_srcs(srcs, srcs_basenames_needing_hashes):
    hashes = []
    for src in srcs:
        parts = src.split(":")
        if len(parts) > 1:
            basename = parts[1]
        else:
            basename = src.split("/")[-1]

        srcs_basenames_needing_hashes[src] = basename
        hashes.append(basename + ".md5")
        hashes.append(basename + ".sha1")
        hashes.append(basename + ".sha512")
    return hashes

# Creates 3+N rules based on the provided targets:
# * A filegroup with just the provided targets (named 'name')
# * A filegroup containing all of the md5, sha1 and sha512 hash files ('name-hashes')
# * A filegroup containing both of the above ('name-and-hashes')
# * All of the necessary md5sum, sha1sum and sha512sum rules
#
# The targets are specified using the srcs and conditioned_srcs attributes.
# srcs is expected to be label list.
# conditioned_srcs is a dictionary mapping conditions to label lists.
#   It will be passed to select().
def release_filegroup(name, srcs = None, conditioned_srcs = None, tags = None, visibility = None, **kwargs):
    if not srcs and not conditioned_srcs:
        fail("srcs and conditioned_srcs cannot both be empty")
    srcs = srcs or []

    # A given src may occur in multiple conditioned_srcs, but we want to create the hash
    # rules only once, so use a dictionary to deduplicate.
    srcs_basenames_needing_hashes = {}

    hashes = _hashes_for_srcs(srcs, srcs_basenames_needing_hashes)
    conditioned_hashes = {}
    if conditioned_srcs:
        for condition, csrcs in conditioned_srcs.items():
            conditioned_hashes[condition] = _hashes_for_srcs(csrcs, srcs_basenames_needing_hashes)

    hash_tags = tags or []
    hash_tags.append("manual")
    for src, basename in srcs_basenames_needing_hashes.items():
        md5sum(name = basename + ".md5", src = src, tags = hash_tags, visibility = visibility)
        sha1sum(name = basename + ".sha1", src = src, tags = hash_tags, visibility = visibility)
        sha512sum(name = basename + ".sha512", src = src, tags = hash_tags, visibility = visibility)

    if conditioned_srcs:
        native.filegroup(
            name = name,
            srcs = srcs + select(conditioned_srcs),
            tags = tags,
            **kwargs
        )
        native.filegroup(
            name = name + "-hashes",
            srcs = hashes + select(conditioned_hashes),
            tags = tags,
            visibility = visibility,
            **kwargs
        )
    else:
        native.filegroup(
            name = name,
            srcs = srcs,
            tags = tags,
            visibility = visibility,
            **kwargs
        )
        native.filegroup(
            name = name + "-hashes",
            srcs = hashes,
            tags = tags,
            visibility = visibility,
            **kwargs
        )

    native.filegroup(
        name = name + "-and-hashes",
        srcs = [name, name + "-hashes"],
        tags = tags,
        visibility = visibility,
        **kwargs
    )
