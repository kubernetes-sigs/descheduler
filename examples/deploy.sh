kubectl create sa descheduler-sa -n kube-system
kubectl create clusterrolebinding descheduler-cluster-role-binding \
    --clusterrole=descheduler-cluster-role \
    --serviceaccount=kube-system:descheduler-sa
kubectl create configmap descheduler-policy-configmap -n kube-system --from-file=policy.yaml
