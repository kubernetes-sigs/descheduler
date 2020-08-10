# Descheduler for Kubernetes

[Descheduler](https://github.com/kubernetes-sigs/descheduler/) for Kubernetes is used to rebalance clusters by evicting pods that can potentially be scheduled on better nodes. In the current implementation, descheduler does not schedule replacement of evicted pods but relies on the default scheduler for that.

## TL;DR:

```shell
helm repo add descheduler https://kubernetes-sigs.github.io/descheduler/
helm install --name my-release descheduler/descheduler-helm-chart
```

## Introduction

This chart bootstraps a [desheduler](https://github.com/kubernetes-sigs/descheduler/) cron job on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

## Prerequisites

- Kubernetes 1.14+

## Installing the Chart

To install the chart with the release name `my-release`:

```shell
helm install --name my-release descheduler/descheduler-helm-chart
```

The command deploys _descheduler_ on the Kubernetes cluster in the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation.

> **Tip**: List all releases using `helm list`

## Uninstalling the Chart

To uninstall/delete the `my-release` deployment:

```shell
helm delete my-release
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Configuration

The following table lists the configurable parameters of the _descheduler_ chart and their default values.

| Parameter                      | Description                                                                                                           | Default                                                |
| ------------------------------ | --------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| `image.repository`             | Docker repository to use                                                                                              | `k8s.gcr.io/descheduler/descheduler`                   |
| `image.tag`                    | Docker tag to use                                                                                                     | `v[chart appVersion]`                                  |
| `image.pullPolicy`             | Docker image pull policy                                                                                              | `IfNotPresent`                                         |
| `nameOverride`                 | String to partially override `descheduler.fullname` template (will prepend the release name)                          | `""`                                                   |
| `fullnameOverride`             | String to fully override `descheduler.fullname` template                                                              | `""`                                                   |
| `schedule`                     | The cron schedule to run the _descheduler_ job on                                                                     | `"*/2 * * * *"`                                        |
| `cmdOptions`                   | The options to pass to the _descheduler_ command                                                                      | _see values.yaml_                                      |
| `deschedulerPolicy.strategies` | The _descheduler_ strategies to apply                                                                                 | _see values.yaml_                                      |
| `priorityClassName`            | The name of the priority class to add to pods                                                                         | `system-cluster-critical`                              |
| `rbac.create`                  | If `true`, create & use RBAC resources                                                                                | `true`                                                 |
| `serviceAccount.create`        | If `true`, create a service account for the cron job                                                                  | `true`                                                 |
| `serviceAccount.name`          | The name of the service account to use, if not set and create is true a name is generated using the fullname template | `nil`                                                  |
