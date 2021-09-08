#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

echo "Make sure that uuid package is installed"

master_uuid=$(uuid)
node1_uuid=$(uuid)
node2_uuid=$(uuid)
kube_apiserver_port=6443
kube_version=1.13.1

DESCHEDULER_ROOT=$(dirname "${BASH_SOURCE}")/../../
E2E_GCE_HOME=$DESCHEDULER_ROOT/hack/e2e-gce


create_cluster() {
	echo "#################### Creating instances ##########################"
	gcloud compute instances create descheduler-$master_uuid --image-family="ubuntu-1804-lts" --image-project="ubuntu-os-cloud" --zone=us-east1-b
	# Keeping the --zone here so as to make sure that e2e's can run locally.
	echo "gcloud compute instances delete descheduler-$master_uuid --zone=us-east1-b --quiet" > $E2E_GCE_HOME/delete_cluster.sh

	gcloud compute instances create descheduler-$node1_uuid --image-family="ubuntu-1804-lts" --image-project="ubuntu-os-cloud" --zone=us-east1-b
	echo "gcloud compute instances delete descheduler-$node1_uuid --zone=us-east1-b --quiet" >> $E2E_GCE_HOME/delete_cluster.sh

	gcloud compute instances create descheduler-$node2_uuid --image-family="ubuntu-1804-lts" --image-project="ubuntu-os-cloud" --zone=us-east1-b
	echo "gcloud compute instances delete descheduler-$node2_uuid --zone=us-east1-c --quiet" >> $E2E_GCE_HOME/delete_cluster.sh

	# Delete the firewall port created for master.
	echo "gcloud compute firewall-rules delete kubeapiserver-$master_uuid --quiet" >> $E2E_GCE_HOME/delete_cluster.sh
	chmod 755 $E2E_GCE_HOME/delete_cluster.sh
}


generate_kubeadm_instance_files() {
	# TODO: Check if they have come up. awk $6 contains the state(RUNNING or not).
	master_private_ip=$(gcloud compute instances list | grep $master_uuid|awk '{print $4}')
	node1_public_ip=$(gcloud compute instances list | grep $node1_uuid|awk '{print $5}')
	node2_public_ip=$(gcloud compute instances list | grep $node2_uuid|awk '{print $5}')
	echo "kubeadm init --kubernetes-version=${kube_version} --apiserver-advertise-address=${master_private_ip}" --ignore-preflight-errors=all --pod-network-cidr=10.96.0.0/12 > $E2E_GCE_HOME/kubeadm_install.sh
}


transfer_install_files() {
	gcloud compute scp $E2E_GCE_HOME/kubeadm_preinstall.sh descheduler-$master_uuid:/tmp --zone=us-east1-b
	gcloud compute scp $E2E_GCE_HOME/kubeadm_install.sh descheduler-$master_uuid:/tmp --zone=us-east1-b
	gcloud compute scp $E2E_GCE_HOME/kubeadm_preinstall.sh descheduler-$node1_uuid:/tmp --zone=us-east1-b
	gcloud compute scp $E2E_GCE_HOME/kubeadm_preinstall.sh descheduler-$node2_uuid:/tmp --zone=us-east1-c
}


install_kube() {
	# Docker installation.
	gcloud compute ssh descheduler-$master_uuid --command "sudo apt-get update; sudo apt-get install -y docker.io" --zone=us-east1-b
	gcloud compute ssh descheduler-$node1_uuid --command "sudo apt-get update; sudo apt-get install -y docker.io" --zone=us-east1-b
	gcloud compute ssh descheduler-$node2_uuid --command "sudo apt-get update; sudo apt-get install -y docker.io" --zone=us-east1-c
	# kubeadm installation.
	# 1. Transfer files to master, nodes.
	transfer_install_files
	# 2. Install kubeadm.
	#TODO: Add rm /tmp/kubeadm_install.sh
	# Open port for kube API server	
	gcloud compute firewall-rules create kubeapiserver-$master_uuid --allow tcp:6443 --source-tags=descheduler-$master_uuid  --source-ranges=0.0.0.0/0 --description="Opening api server port"

	gcloud compute ssh descheduler-$master_uuid --command "sudo chmod 755 /tmp/kubeadm_preinstall.sh; sudo /tmp/kubeadm_preinstall.sh" --zone=us-east1-b
	kubeadm_join_command=$(gcloud compute ssh descheduler-$master_uuid --command "sudo chmod 755 /tmp/kubeadm_install.sh; sudo /tmp/kubeadm_install.sh" --zone=us-east1-b|grep 'kubeadm join')
	
	# Copy the kubeconfig file onto /tmp for e2e tests.
	gcloud compute ssh descheduler-$master_uuid --command "sudo cp /etc/kubernetes/admin.conf /tmp; sudo chmod 777 /tmp/admin.conf" --zone=us-east1-b
	gcloud compute scp descheduler-$master_uuid:/tmp/admin.conf /tmp/admin.conf --zone=us-east1-b

	# Postinstall on master, need to add a network plugin for kube-dns to come to running state.
	gcloud compute ssh descheduler-$master_uuid --command "sudo kubectl apply -f https://raw.githubusercontent.com/cloudnativelabs/kube-router/master/daemonset/kubeadm-kuberouter.yaml --kubeconfig /etc/kubernetes/admin.conf" --zone=us-east1-b
	echo $kubeadm_join_command > $E2E_GCE_HOME/kubeadm_join.sh

	# Copy kubeadm_join to every node.
	#TODO: Put these in a loop, so that extension becomes possible.	
	gcloud compute ssh descheduler-$node1_uuid --command "sudo chmod 755 /tmp/kubeadm_preinstall.sh; sudo /tmp/kubeadm_preinstall.sh" --zone=us-east1-b
	gcloud compute scp $E2E_GCE_HOME/kubeadm_join.sh descheduler-$node1_uuid:/tmp --zone=us-east1-b
	gcloud compute ssh descheduler-$node1_uuid --command "sudo chmod 755 /tmp/kubeadm_join.sh; sudo /tmp/kubeadm_join.sh" --zone=us-east1-b
	
	gcloud compute ssh descheduler-$node2_uuid --command "sudo chmod 755 /tmp/kubeadm_preinstall.sh; sudo /tmp/kubeadm_preinstall.sh" --zone=us-east1-c
	gcloud compute scp $E2E_GCE_HOME/kubeadm_join.sh descheduler-$node2_uuid:/tmp --zone=us-east1-c
	gcloud compute ssh descheduler-$node2_uuid --command "sudo chmod 755 /tmp/kubeadm_join.sh; sudo /tmp/kubeadm_join.sh" --zone=us-east1-c
}


create_cluster

generate_kubeadm_instance_files

install_kube
