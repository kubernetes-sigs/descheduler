 Had to copy generated files from kube1.7 into vendor directory as I was not able to create the generated files. Especially, 
- "zz_generated.openapi.go" to vendor/k8s.io/kubernetes/pkg/generated/openapi.
- "bindata.go" to vendor/k8s.io/kubernetes/test/e2e/generated

After that

- cd <descheduler_home>/test/e2e/
- ./test-e2e.sh
