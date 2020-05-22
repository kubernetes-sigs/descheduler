# Release Guide

## Semi-automatic

1. Make sure your repo is clean by git's standards
2. Create a release branch `git checkout -b release-1.18` (not required for patch releases)
3. Push the release branch to the descheuler repo and ensure branch protection is enabled (not required for patch releases)
4. Tag the repository and push the tag `VERSION=v0.18.0 git tag -m $VERSION $VERSION; git push origin $VERSION`
5. Publish a draft release using the tag you just created
6. Perform the [image promotion process](https://github.com/kubernetes/k8s.io/tree/master/k8s.gcr.io#image-promoter)
7. Publish release
8. Email `kubernetes-sig-scheduling@googlegroups.com` to announce the release

## Manual

1. Make sure your repo is clean by git's standards
2. Create a release branch `git checkout -b release-1.18` (not required for patch releases)
3. Push the release branch to the descheuler repo and ensure branch protection is enabled (not required for patch releases)
4. Tag the repository and push the tag `VERSION=v0.18.0 git tag -m $VERSION $VERSION; git push origin $VERSION`
5. Checkout the tag you just created and make sure your repo is clean by git's standards `git checkout $VERSION`
6. Build and push the container image to the staging registry `VERSION=$VERSION make push`
7. Publish a draft release using the tag you just created
8. Perform the [image promotion process](https://github.com/kubernetes/k8s.io/tree/master/k8s.gcr.io#image-promoter)
9. Publish release
10. Email `kubernetes-sig-scheduling@googlegroups.com` to announce the release

## Notes
See [post-descheduler-push-images dashboard](https://testgrid.k8s.io/sig-scheduling#post-descheduler-push-images) for staging registry image build job status.

View the descheduler staging registry using [this URL](https://console.cloud.google.com/gcr/images/k8s-staging-descheduler/GLOBAL/descheduler) in a web browser
or use the below `gcloud` commands.

List images in staging registry.
```
gcloud container images list --repository gcr.io/k8s-staging-descheduler
```

List descheduler image tags in the staging registry.
```
gcloud container images list-tags gcr.io/k8s-staging-descheduler/descheduler
```

Get SHA256 hash for a specific image in the staging registry.
```
gcloud container images describe gcr.io/k8s-staging-descheduler/descheduler:v20200206-0.9.0-94-ge2a23f284
```

Pull image from the staging registry.
```
docker pull gcr.io/k8s-staging-descheduler/descheduler:v20200206-0.9.0-94-ge2a23f284
```
