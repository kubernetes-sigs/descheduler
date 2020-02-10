## Contributor Guide

### Build and Run

- Checkout the repo into your $GOPATH directory under src/sigs.k8s.io/descheduler

Build descheduler:

```sh
$ make
```

and run descheduler:

```sh
$ ./_output/bin/descheduler --kubeconfig <path to kubeconfig> --policy-config-file <path-to-policy-file>
```

If you want more information about what descheduler is doing add `-v 1` to the command line

For more information about available options run:
```
$ ./_output/bin/descheduler --help
```