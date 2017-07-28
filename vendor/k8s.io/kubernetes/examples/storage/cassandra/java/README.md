# Cassandra on Kubernetes Custom Seed Provider: releases.k8s.io/HEAD

Within any deployment of Cassandra a Seed Provider is used to for node discovery and communication.  When a Cassandra node first starts it must discover which nodes, or seeds, for the information about the Cassandra nodes in the ring / rack / datacenter.

This Java project provides a custom Seed Provider which communicates with the Kubernetes API to discover the required information.  This provider is bundled with the Docker provided in this example.

# Configuring the Seed Provider

The following environment variables may be used to override the default configurations:

| ENV VAR       | DEFAULT VALUE  | NOTES |
| ------------- |:-------------: |:-------------:|
| KUBERNETES_PORT_443_TCP_ADDR   | kubernetes.default.svc.cluster.local  | The hostname of the API server   |
| KUBERNETES_PORT_443_TCP_PORT   | 443                                   | API port number                  |
| CASSANDRA_SERVICE              | cassandra                             | Default service name for lookup  |
| POD_NAMESPACE                  | default                               | Default pod service namespace    |
| K8S_ACCOUNT_TOKEN 		 | /var/run/secrets/kubernetes.io/serviceaccount/token | Default path to service token |

# Using


If no endpoints are discovered from the API the seeds configured in the cassandra.yaml file are used.

# Provider limitations

This Cassandra Provider implements `SeedProvider`. and utilizes `SimpleSnitch`.  This limits a Cassandra Ring to a single Cassandra Datacenter and ignores Rack setup.  Datastax provides more documentation on the use of [_SNITCHES_](https://docs.datastax.com/en/cassandra/3.x/cassandra/architecture/archSnitchesAbout.html).  Further development is planned to
expand this capability.

This in affect makes every node a seed provider, which is not a recommended best practice.  This increases maintenance and reduces gossip performance.


<!-- BEGIN MUNGE: GENERATED_ANALYTICS -->
[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/examples/storage/cassandra/java/README.md?pixel)]()
<!-- END MUNGE: GENERATED_ANALYTICS -->
