# Descheduler Design Constraints

This is a slowly growing document that lists good practices, conventions, and design decisions.

## Overview

TBD

## Code convention

* *formatting code*: running `make fmt` before committing each change to avoid ci failing

## Unit Test Conventions

These are the known conventions that are useful to practice whenever reasonable:

* *single pod creation*: each pod variable built using `test.BuildTestPod` is updated only through the `apply` argument of `BuildTestPod`
* *single node creation*: each node variable built using `test.BuildTestNode` is updated only through the `apply` argument of `BuildTestNode`
* *no object instance sharing*: each object built through `test.BuildXXX` functions is newly created in each unit test to avoid accidental object mutations
* *no object instance duplication*: avoid duplication by no creating two objects with the same passed values at two different places. E.g. two nodes created with the same memory, cpu and pods requests. Rather create a single function wrapping test.BuildTestNode and invoke this wrapper multiple times.

The aim is to reduce cognitive load when reading and debugging the test code.

## Design Decisions FAQ

This section documents common questions about design decisions in the descheduler codebase and the rationale behind them.

### Why doesn't the framework provide helpers for registering and retrieving indexers for plugins?

In general, each plugin can have many indexers—for example, for nodes, namespaces, pods, and other resources. Each plugin, depending on its internal optimizations, may choose a different indexing function. Indexers are currently used very rarely in the framework and default plugins. Therefore, extending the framework interface with additional helpers for registering and retrieving indexers might introduce an unnecessary and overly restrictive layer without first understanding how indexers will be used. For the moment, I suggest avoiding any restrictions on how many indexers can be registered or which ones can be registered. Instead, we should extend the framework handle to provide a unique ID for each profile, so that indexers within the same profile share a unique prefix. This avoids collisions when the same profile is instantiated more than once. Later, once we learn more about indexer usage, we can revisit whether it makes sense to impose additional restrictions.
