# Release process

## Semi-automatic

1. Make sure your repo is clean by git's standards
2. Tag the repository and push the tag `VERSION=v0.10.0 git tag -m $VERSION $VERSION; git push origin $VERSION`
3. Publish a draft release using the tag you just created
4. Perform the [image promotion process](https://github.com/kubernetes/k8s.io/tree/master/k8s.gcr.io#image-promoter)
5. Publish release
6. Email `kubernetes-sig-scheduling@googlegroups.com` to announce the release

## Manual

1. Make sure your repo is clean by git's standards
2. Tag the repository and push the tag `VERSION=v0.10.0 git tag -m $VERSION $VERSION; git push origin $VERSION`
3. Checkout the tag you just created and make sure your repo is clean by git's standards `git checkout $VERSION`
4. Build and push the container image to the staging registry `VERSION=$VERSION make push`
5. Publish a draft release using the tag you just created
6. Perform the [image promotion process](https://github.com/kubernetes/k8s.io/tree/master/k8s.gcr.io#image-promoter)
7. Publish release
8. Email `kubernetes-sig-scheduling@googlegroups.com` to announce the release

## Notes
See [post-descheduler-push-images dashboard](https://testgrid.k8s.io/sig-scheduling#post-descheduler-push-images) for staging registry image build job status.

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
