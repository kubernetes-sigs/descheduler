# Release Guide

## Container Image

### Semi-automatic

1. Make sure your repo is clean by git's standards
2. Create a release branch `git checkout -b release-1.18` (not required for patch releases)
3. Push the release branch to the descheuler repo and ensure branch protection is enabled (not required for patch releases)
4. Tag the repository and push the tag `VERSION=v0.18.0 git tag -m $VERSION $VERSION; git push origin $VERSION`
5. Publish a draft release using the tag you just created
6. Perform the [image promotion process](https://github.com/kubernetes/k8s.io/tree/master/k8s.gcr.io#image-promoter)
7. Publish release
8. Email `kubernetes-sig-scheduling@googlegroups.com` to announce the release

### Manual

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

### Notes
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

## Helm Chart
Helm chart releases are managed by a separate set of git tags that are prefixed with `chart-*`. Example git tag name is `chart-0.18.0`. Released versions of the
helm charts are stored in the `gh-pages` branch of this repo. The [chart-releaser-action GitHub Action](https://github.com/helm/chart-releaser-action) is setup to
build and push the helm charts to the `gh-pages` branch when a `chart-*` git tag is created.

The major and minor version of the chart matches the descheduler major and minor versions. For example descheduler helm chart version chart-0.18.0 corresponds
to descheduler version v0.18.0. The patch version of the descheduler helm chart and the patcher version of the descheduler will not necessarily match. The patch
version of the descheduler helm chart is used to version changes specific to the helm chart.

1. Merge all helm chart changes into the appropriate release branch(i.e. release-1.18)
   1. Ensure that `appVersion` in file `charts/descheduler/Chart.yaml` matches the descheduler version(no `v` prefix)
   2. Ensure that `version` in file `charts/descheduler/Chart.yaml` has been incremented. This is the chart version.
2. Make sure your repo is clean by git's standards
3. Create the tag and push it `git checkout release-1.18; CHART_VERSION=chart-0.18.0; git tag $CHART_VERSION; git push origin $CHART_VERSION`
4. Verify the new helm artifact has been successfully pushed to the `gh-pages` branch
