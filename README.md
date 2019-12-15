
#  Load Rebalancing of Cloud Computing Services

## Introduction
This project is based on kubernetes and carries out the research and development of load rebalancing based on cloud computing services. Service containerization has become well known as the mainstream deployment of distributed services. Kubernetes is the mainstream container arrangement tool, which provides good dynamic scaling, deployment, release and other functions for containerized services. However, until now, the load balancing of distributed services is still a part that needs to be continuously optimized.We need to work out a scheme that can guarantee both the dynamic scalability of the service and the dynamic load balancing of the service.

This project consists of two parts, control module and work module. This Github project is the work moduler.

## Existing Problems 

1 The current load balancing solution offered by the kubernetes platform is only for initial deployment of services. However, service deployment is not automatically scheduled without exceptions. This situation can lead to a build-up of load, with the application services unable to be allocated to the new  physical resources that are later added, resulting in an unbalanced load situation.

2 For service applications with storage, scheduling can result in the loss of stored content within the container, including data in memory and on disk.

3 The existing kubernetes cluster solution cannot dynamically adjust the security load of the cluster resources.

## Research Platform

linux

## Programming Language

go, Java, C++.

## Research Topic

1 The dynamic scheduling scheme of containerized services with storage

2 Dynamic scheduling and deployment solutions for containerized services based on cluster resources

## Build and Run
Build descheduler:

$ make
and run descheduler:

$ ./_output/bin/descheduler --kubeconfig <path to kubeconfig> --policy-config-file <path-to-policy-file>
If you want more information about what descheduler is doing add -v 1 to the command line

For more information about available options run:

$ ./_output/bin/descheduler --help

## Tips：

1 In duplicates strategies, the rules for detecting whether the pod in the same deployment has been deployed on the same node，which can be tailored to individual needs: to modify FindDuplicatePods

2 In LowNodeUtilization strategies, The sorting method of nodes is in method: SortNodesByUsage，The sorting rule is the sum of the three percentages of CPU, MEM and PODs, which can be modified according to individual needs.

3 In LowNodeUtilization strategies, no verification of Pod label is added.

4 The policy file cannot be generated dynamically, so you can increase the threshold in the controller module control policy according to your own needs.

5 When pod on the maximum load node is expelled, You must consider whether the pod exists on the maximum load node you take.Otherwise the node will never expel the pod. 

