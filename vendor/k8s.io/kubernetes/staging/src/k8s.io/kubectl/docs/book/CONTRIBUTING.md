# Contributing

## Process

### Fixing Issues

1. Open an Issue
1. Create a PR
1. Email PR to sig-cli@googlegroups.com with subject `Kubectl Book: Fix Issue <Issue> in <PR>`
1. Optional: Come to sig-cli meeting to discuss

### Adding New Content

1. Open an Issue with proposed content
1. Email sig-cli@googlegroups.com with subject `Kubectl Book: Proposed Content <Issue>`
1. Optional: Come to sig-cli meeting to discuss

## Editing

### Running Locally

- Install [GitBook Toolchain](https://toolchain.gitbook.com/setup.html)
- From `docs/book` run `npm install`  to install node_modules locally (don't run install, it updates the shrinkwrap.json)
- From `docs/book` run `gitbook serve`
- Go to `http://localhost:4000` in a browser

### Adding a Section

- Update `SUMMARY.md` with a new section formatted as `## Section Name`

### Adding a Chapter

- Update `SUMMARY.md` under section with chapter formatted as `* [Name of Chapter](pages/section_chapter.md)`
- Add file `pages/section_chapter.md`

### Adding Examples to a Chapter

```bash
{% method %}
Text Explaining Example
{% sample lang="yaml" %}
Formatted code
{% endmethod %}
```

### Adding Notes to a Chapter

```bash
{% panel style="info", title="Title of Note" %}
Note text
{% endpanel %}
```

Notes may have the following styles:

- success
- info
- warning
- danger

### Building and Publishing a release

- Run `gitbook build`
- Push fies in `_book` to a server

### Adding GitBook plugins

- Update `book.json` with the plugin
- Run `npm install <npm-plugin-name>`

### Cool plugins

See https://github.com/swapagarwal/awesome-gitbook-plugins for more plugins.