# Descheduler for Kubernetes

[Descheduler](https://github.com/kubernetes-sigs/descheduler/) for Kubernetes is used to rebalance clusters by evicting pods that can potentially be scheduled on better nodes. In the current implementation, descheduler does not schedule replacement of evicted pods but relies on the default scheduler for that.

## TL;DR:

```shell
helm repo add descheduler https://kubernetes-sigs.github.io/descheduler/
helm install my-release --namespace kube-system descheduler/descheduler
```

## Introduction

This chart bootstraps a [descheduler](https://github.com/kubernetes-sigs/descheduler/) cron job on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager.

## Prerequisites

- Kubernetes 1.14+

## Installing the Chart

To install the chart with the release name `my-release`:

```shell
helm install --namespace kube-system my-release descheduler/descheduler
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

| Parameter                           | Description                                                                                                           | Default                                   |
| ----------------------------------- | --------------------------------------------------------------------------------------------------------------------- | ----------------------------------------- |
| `kind`                              | Use as CronJob or Deployment                                                                                          | `CronJob`                                 |
| `image.repository`                  | Docker repository to use                                                                                              | `registry.k8s.io/descheduler/descheduler` |
| `image.tag`                         | Docker tag to use                                                                                                     | `v[chart appVersion]`                     |
| `image.pullPolicy`                  | Docker image pull policy                                                                                              | `IfNotPresent`                            |
| `imagePullSecrets`                  | Docker repository secrets                                                                                             | `[]`                                      |
| `nameOverride`                      | String to partially override `descheduler.fullname` template (will prepend the release name)                          | `""`                                      |
| `fullnameOverride`                  | String to fully override `descheduler.fullname` template                                                              | `""`                                      |
| `namespaceOverride`                 | Override the deployment namespace; defaults to .Release.Namespace                                                     | `""`                                      |
| `cronJobApiVersion`                 | CronJob API Group Version                                                                                             | `"batch/v1"`                              |
| `schedule`                          | The cron schedule to run the _descheduler_ job on                                                                     | `"*/2 * * * *"`                           |
| `startingDeadlineSeconds`           | If set, configure `startingDeadlineSeconds` for the _descheduler_ job                                                 | `nil`                                     |
| `timeZone`                          | configure `timeZone` for CronJob                                                                                      | `nil`                                     |
| `successfulJobsHistoryLimit`        | If set, configure `successfulJobsHistoryLimit` for the _descheduler_ job                                              | `3`                                       |
| `failedJobsHistoryLimit`            | If set, configure `failedJobsHistoryLimit` for the _descheduler_ job                                                  | `1`                                       |
| `ttlSecondsAfterFinished`           | If set, configure `ttlSecondsAfterFinished` for the _descheduler_ job                                                 | `nil`                                     |
| `deschedulingInterval`              | If using kind:Deployment, sets time between consecutive descheduler executions.                                       | `5m`                                      |
| `replicas`                          | The replica count for Deployment                                                                                      | `1`                                       |
| `leaderElection`                    | The options for high availability when running replicated components                                                  | _see values.yaml_                         |
| `cmdOptions`                        | The options to pass to the _descheduler_ command                                                                      | _see values.yaml_                         |
| `priorityClassName`                 | The name of the priority class to add to pods                                                                         | `system-cluster-critical`                 |
| `rbac.create`                       | If `true`, create & use RBAC resources                                                                                | `true`                                    |
| `resources`                         | Descheduler container CPU and memory requests/limits                                                                  | _see values.yaml_                         |
| `serviceAccount.create`             | If `true`, create a service account for the cron job                                                                  | `true`                                    |
| `serviceAccount.name`               | The name of the service account to use, if not set and create is true a name is generated using the fullname template | `nil`                                     |
| `serviceAccount.annotations`        | Specifies custom annotations for the serviceAccount                                                                   | `{}`                                      |
| `podAnnotations`                    | Annotations to add to the descheduler Pods                                                                            | `{}`                                      |
| `podLabels`                         | Labels to add to the descheduler Pods                                                                                 | `{}`                                      |
| `nodeSelector`                      | Node selectors to run the descheduler cronjob/deployment on specific nodes                                            | `nil`                                     |
| `service.enabled`                   | If `true`, create a service for deployment                                                                            | `false`                                   |
| `serviceMonitor.enabled`            | If `true`, create a ServiceMonitor for deployment                                                                     | `false`                                   |
| `serviceMonitor.namespace`          | The namespace where Prometheus expects to find service monitors                                                       | `nil`                                     |
| `serviceMonitor.additionalLabels`   | Add custom labels to the ServiceMonitor resource                                                                      | `{}`                                      |
| `serviceMonitor.interval`           | The scrape interval. If not set, the Prometheus default scrape interval is used                                       | `nil`                                     |
| `serviceMonitor.honorLabels`        | Keeps the scraped data's labels when labels are on collisions with target labels.                                     | `true`                                    |
| `serviceMonitor.insecureSkipVerify` | Skip TLS certificate validation when scraping                                                                         | `true`                                    |
| `serviceMonitor.serverName`         | Name of the server to use when validating TLS certificate                                                             | `nil`                                     |
| `serviceMonitor.metricRelabelings`  | MetricRelabelConfigs to apply to samples after scraping, but before ingestion                                         | `[]`                                      |
| `serviceMonitor.relabelings`        | RelabelConfigs to apply to samples before scraping                                                                    | `[]`                                      |
| `affinity`                          | Node affinity to run the descheduler cronjob/deployment on specific nodes                                             | `nil`                                     |
| `topologySpreadConstraints`         | Topology Spread Constraints to spread the descheduler cronjob/deployment across the cluster                           | `[]`                                     |
| `tolerations`                       | tolerations to run the descheduler cronjob/deployment on specific nodes                                               | `nil`                                     |
| `suspend`                           | Set spec.suspend in descheduler cronjob                                                                               | `false`                                   |
| `commonLabels`                      | Labels to apply to all resources                                                                                      | `{}`                                      |
| `livenessProbe`                     | Liveness probe configuration for the descheduler container                                                            | _see values.yaml_                         |
