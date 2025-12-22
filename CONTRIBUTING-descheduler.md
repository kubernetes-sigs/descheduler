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
