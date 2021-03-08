# Markdown Table of Contents Generator

`mdtoc` is a utility for generating a table-of-contents for markdown files.

Only github-flavored markdown is currently supported, but I am open to accepting patches to add
other formats.

# Table of Contents

Generated with `mdtoc --inplace README.md`

<!-- toc -->
- [Usage](#usage)
- [Installation](#installation)
- [Community, discussion, contribution, and support](#community-discussion-contribution-and-support)
  - [Code of conduct](#code-of-conduct)
<!-- /toc -->

## Usage

Usage: `mdtoc [OPTIONS] [FILE]...`
Generate a table of contents for a markdown file (github flavor).

TOC may be wrapped in a pair of tags to allow in-place updates:
```
<!-- toc -->
generated TOC goes here
<!-- /toc -->
```

TOC indentation is normalized, so the shallowest header has indentation 0.

**Options:**

`--dryrun` - Whether to check for changes to TOC, rather than overwriting.
Requires `--inplace` flag. Exit code 1 if there are changes.

`--inplace` - Whether to edit the file in-place, or output to STDOUT. Requires
toc tags to be present.

`--skip-prefix` - Whether to ignore any headers before the opening toc
tag. (default true)

For example, with `--skip-prefix=false` the TOC for this file becomes:

```
- [Markdown Table of Contents Generator](#markdown-table-of-contents-generator)
- [Table of Contents](#table-of-contents)
  - [Usage](#usage)
  - [Installation](#installation)
```

## Installation

On linux, simply download and run the [standalone release
binary](https://github.com/kubernetes-sigs/mdtoc/releases)

```sh
# Optional: Verify the file integrity - check the release notes for the expected value.
$ sha256sum $BINARY
$ chmod +x $BINARY
```

Or, if you have a go development environment set up:

```
go get sigs.k8s.io/mdtoc
```

## Community, discussion, contribution, and support

Learn how to engage with the Kubernetes community on the [community page](http://kubernetes.io/community/).

You can reach the maintainers of this project at:

- [Slack](http://slack.k8s.io/)
- [Mailing List](https://groups.google.com/forum/#!forum/kubernetes-dev)

### Code of conduct

Participation in the Kubernetes community is governed by the [Kubernetes Code of Conduct](code-of-conduct.md).

[owners]: https://git.k8s.io/community/contributors/guide/owners.md
[Creative Commons 4.0]: https://git.k8s.io/website/LICENSE
