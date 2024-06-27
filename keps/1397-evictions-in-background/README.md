<!--
**Note:** When your KEP is complete, all of these comment blocks should be removed.

To get started with this template:

- [ ] **Pick a hosting SIG.**
  Make sure that the problem space is something the SIG is interested in taking
  up. KEPs should not be checked in without a sponsoring SIG.
- [ ] **Create an issue in kubernetes/enhancements**
  When filing an enhancement tracking issue, please make sure to complete all
  fields in that template. One of the fields asks for a link to the KEP. You
  can leave that blank until this KEP is filed, and then go back to the
  enhancement and add the link.
- [ ] **Make a copy of this template directory.**
  Copy this template into the owning SIG's directory and name it
  `NNNN-short-descriptive-title`, where `NNNN` is the issue number (with no
  leading-zero padding) assigned to your enhancement above.
- [ ] **Fill out as much of the kep.yaml file as you can.**
  At minimum, you should fill in the "Title", "Authors", "Owning-sig",
  "Status", and date-related fields.
- [ ] **Fill out this file as best you can.**
  At minimum, you should fill in the "Summary" and "Motivation" sections.
  These should be easy if you've preflighted the idea of the KEP with the
  appropriate SIG(s).
- [ ] **Create a PR for this KEP.**
  Assign it to people in the SIG who are sponsoring this process.
- [ ] **Merge early and iterate.**
  Avoid getting hung up on specific details and instead aim to get the goals of
  the KEP clarified and merged quickly. The best way to do this is to just
  start with the high-level sections and fill out details incrementally in
  subsequent PRs.

Just because a KEP is merged does not mean it is complete or approved. Any KEP
marked as `provisional` is a working document and subject to change. You can
denote sections that are under active debate as follows:

```
<<[UNRESOLVED optional short context or usernames ]>>
Stuff that is being argued.
<<[/UNRESOLVED]>>
```

When editing KEPS, aim for tightly-scoped, single-topic PRs to keep discussions
focused. If you disagree with what is already in a document, open a new PR
with suggested changes.

One KEP corresponds to one "feature" or "enhancement" for its whole lifecycle.
You do not need a new KEP to move from beta to GA, for example. If
new details emerge that belong in the KEP, edit the KEP. Once a feature has become
"implemented", major changes should get new KEPs.

The canonical place for the latest set of instructions (and the likely source
of this file) is [here](/keps/NNNN-kep-template/README.md).

**Note:** Any PRs to move a KEP to `implementable`, or significant changes once
it is marked `implementable`, must be approved by each of the KEP approvers.
If none of those approvers are still appropriate, then changes to that list
should be approved by the remaining approvers and/or the owning SIG (or
SIG Architecture for cross-cutting KEPs).
-->
# KEP-1397: descheduler integration with evacuation API as an alternative to eviction API

<!--
This is the title of your KEP. Keep it short, simple, and descriptive. A good
title can help communicate what the KEP is and should be considered as part of
any review.
-->

<!--
A table of contents is helpful for quickly jumping to sections of a KEP and for
highlighting any additional information provided beyond the standard KEP
template.

Ensure the TOC is wrapped with
  <code>&lt;!-- toc --&rt;&lt;!-- /toc --&rt;</code>
tags, and then generate with `hack/update-toc.sh`.
-->

<!-- toc -->
- [Release Signoff Checklist](#release-signoff-checklist)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Proposal](#proposal)
  - [Expected Outcomes](#expected-outcomes)
  - [User Stories (Optional)](#user-stories-optional)
  - [Annotation vs. evacuation API based eviction](#annotation-vs-evacuation-api-based-eviction)
  - [Workflow Description](#workflow-description)
    - [Actors](#actors)
    - [Workflow Steps](#workflow-steps)
  - [Notes/Constraints/Caveats (Optional)](#notesconstraintscaveats-optional)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Design Details](#design-details)
  - [Implementation Strategies](#implementation-strategies)
    - [API changes](#api-changes)
    - [v1alpha1](#v1alpha1)
    - [v1alphaN or beta](#v1alphan-or-beta)
    - [Code changes](#code-changes)
    - [Metrics](#metrics)
  - [Open Questions [optional]](#open-questions-optional)
  - [Test Plan](#test-plan)
      - [Prerequisite testing updates](#prerequisite-testing-updates)
      - [Unit tests](#unit-tests)
      - [Integration tests](#integration-tests)
      - [e2e tests](#e2e-tests)
  - [Graduation Criteria](#graduation-criteria)
    - [Alpha](#alpha)
    - [Beta](#beta)
    - [GA](#ga)
  - [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy)
  - [Version Skew Strategy](#version-skew-strategy)
- [Production Readiness Review Questionnaire](#production-readiness-review-questionnaire)
  - [Feature Enablement and Rollback](#feature-enablement-and-rollback)
  - [Rollout, Upgrade and Rollback Planning](#rollout-upgrade-and-rollback-planning)
  - [Monitoring Requirements](#monitoring-requirements)
  - [Dependencies](#dependencies)
  - [Scalability](#scalability)
  - [Troubleshooting](#troubleshooting)
- [Implementation History](#implementation-history)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)
- [Infrastructure Needed (Optional)](#infrastructure-needed-optional)
<!-- /toc -->

## Release Signoff Checklist

<!--
**ACTION REQUIRED:** In order to merge code into a release, there must be an
issue in [kubernetes/enhancements] referencing this KEP and targeting a release
milestone **before the [Enhancement Freeze](https://git.k8s.io/sig-release/releases)
of the targeted release**.

For enhancements that make changes to code or processes/procedures in core
Kubernetes—i.e., [kubernetes/kubernetes], we require the following Release
Signoff checklist to be completed.

Check these off as they are completed for the Release Team to track. These
checklist items _must_ be updated for the enhancement to be released.
-->

Items marked with (R) are required *prior to targeting to a milestone / release*.

- [ ] (R) Enhancement issue in release milestone, which links to KEP dir in [kubernetes/enhancements] (not the initial KEP PR)
- [ ] (R) KEP approvers have approved the KEP status as `implementable`
- [ ] (R) Design details are appropriately documented
- [ ] (R) Test plan is in place, giving consideration to SIG Architecture and SIG Testing input (including test refactors)
  - [ ] e2e Tests for all Beta API Operations (endpoints)
  - [ ] (R) Ensure GA e2e tests meet requirements for [Conformance Tests](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/conformance-tests.md)
  - [ ] (R) Minimum Two Week Window for GA e2e tests to prove flake free
- [ ] (R) Graduation criteria is in place
  - [ ] (R) [all GA Endpoints](https://github.com/kubernetes/community/pull/1806) must be hit by [Conformance Tests](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/conformance-tests.md)
- [ ] (R) Production readiness review completed
- [ ] (R) Production readiness review approved
- [ ] "Implementation History" section is up-to-date for milestone
- [ ] User-facing documentation has been created in [kubernetes/website], for publication to [kubernetes.io]
- [ ] Supporting documentation—e.g., additional design documents, links to mailing list discussions/SIG meetings, relevant PRs/issues, release notes

<!--
**Note:** This checklist is iterative and should be reviewed and updated every time this enhancement is being considered for a milestone.
-->

[kubernetes.io]: https://kubernetes.io/
[kubernetes/enhancements]: https://git.k8s.io/enhancements
[kubernetes/kubernetes]: https://git.k8s.io/kubernetes
[kubernetes/website]: https://git.k8s.io/website

## Summary

The descheduler eviction policy is built on top of the eviction API.
The API currently does not support eviction requests that are not completed right away.
Instead, any eviction needs to either succeed or be rejected in response.
Nevertheless, there are cases where an eviction request is expected to only initiate eviction.
While getting confirmation or rejection of the eviction initiation (or its promise).

The descheduler treats all pods as cattle.
Each descheduling loop consists of  pods nomination followed by their eviction.
When eviction of one pod does not succeed another pod is selected until a limit
on the number of evictions is reached or all nominated pods are evicted.
The descheduler does not keep track of pods that failed to be evicted.
The next descheduling loop repeats the same routine.
The order of eviction is not guaranteed.
Thus, any subsequent loop can either evict pods that failed to be evicted
in the previous loop or completely ignore them if a limit is reached.
Thus, if an administrator installs a webhook that intercepts eviction requests
while changing the underlying functionality (e.g. eviction in background
initiation but returning an error code instead of success),
the descheduler can do more harm than good.

## Motivation

Kubernetes ecosystem is wide.
There are various types of workloads in various environments that may require
different eviction policies to be applied. Some pods can be evicted right away,
some pods may need more time for clean up. Other pods may need more time
than a graceful termination period gives or be able to retry eviction
in case a live migration did not succeed.

### Goals

- **Enhance Descheduler eviction policy**: Allow pods that require eviction
  without getting evicted right away to get a promise from third part
  component on handling the eviction without directly invoking the upstream eviction API.
  Eventually replacing the eviction API with a new [evacuation API](https://github.com/kubernetes/enhancements/pull/4565).
- **No more evictions, only requests for evacuation**: the descheduler will
  no longer guarantee eviction. All the configurable limits per namespace/node/run will
  change their semantics to a maximal number of evacuation requests per namespace/node/run.
- **New sorting algorithm to prioritize in-progress evictions**: Sort the nominated pods
  to prefer those with eviction already in progress (or its promise)
  to minimize workload disruptions.
- **New prefilter algorithm to exclude in-progress evictions**: Filter out all pods
  that have an already existing evacuation request present.
- **Measure pending evictions**: New metric to keep count of evictions in progress/evacuation requests

### Non-Goals

- Implementation of any customized eviction policy: The Descheduler will only
  acknowledge existence of pods that require different eviction handling.
  The actual eviction (e.g. live migration) will be implemented by third parties.

## Proposal

This enhancement proposes the implementation of a more informed
eviction policy for the Descheduler.
Allowing plugins (constructing lists of pods nominated for eviction)
to be aware of pods with eviction in progress to avoid evicting
more than a configured limit allows.
Reducing unnecessary evictions can improve overall workload
distribution and save cost of bringing up new pods.

### Expected Outcomes

- Live migration implemented through external admission webhooks
  are no longer perceived as eviction errors.
  Currently, no pod with a live migration in background is evicted properly.
  Thus resulting in eviction of all such pods from all possible nodes
  until a limit is reached. Which is an undesirable behaviour.
- Descheduling plugins are more aware of pods with live migration in background.
  Thus, improving the decision making behind which pods get nominated for eviction.
- The descheduler will no longer be responsible for direct eviction.
  Instead, only requesting eviction and letting other components (more
  informed and following various policies) to perform the actual eviction.

### User Stories (Optional)

- As a cluster admin running a K8s cluster with a descheduler,
  I want to evict KubeVirt pods that may require live migration of VMs
  before a pod can be  evicted. A VM live migration can take minutes
  to complete or may require retries.
- As an end user deploying my application on a K8s cluster with a descheduler,
  I want to evict pods while performing pod live migration to persist
  pod's state without storing any necessary data into pod-independent persistent storage.
- As a developer I want to be able to implement a custom eviction policy to
  address various company use cases that are not supported
  by default when running a descheduler instance.
- As a descheduler plugin developer I want to be aware of evictions
  in progress to improve pod nomination and thus avoid unnecessary disruptions.
- As a security professional I want to make sure any live migration
  adhers to security policies and protects sensitive data.

### Annotation vs. evacuation API based eviction

The [evacuation API](https://github.com/kubernetes/enhancements/pull/4565) is expected
to replace the [eviction API](https://kubernetes.io/docs/concepts/scheduling-eviction/api-eviction/).
Instead of checking whether an eviction was successful or failed, an evacuation request is created.
In case the request fails to be created a pod eviction is skipped as previously.
In addition, each cycle will keep a number of evacuation requests created
instead of a number of pods evicted.

The annotation based approach is a special case of invoking the eviction API:

|  | Eviction API | Evacuation API |
|--|--|--|
|**Eviction handling**|Eviction of a pod with `descheduler.alpha.kubernetes.io/request-evict-only` annotation. Checking the eviction error, return code and response text. Interpreting a specific error code and a presence of a known suffix in the response text. |Creation of an evacuation CR for the given pod. Checking the evacuation CR creation error.|
|**Pod nomination and sorting**|Preferring pods with both `descheduler.alpha.kubernetes.io/request-evict-only` and `descheduler.alpha.kubernetes.io/eviction-in-progress` annotations.|Preferring pods that have a corresponding evacuation CR present.|
|**Cache reconstruction**|When a descheduler gets restarted and a new internal caches (with tracked annotations) cleared the descheduler lists all pods and populate the cache with any pod that has both annotation (`descheduler.alpha.kubernetes.io/request-evict-only` and `descheduler.alpha.kubernetes.io/eviction-in-progress`) present. In case only the first annotation is present but the eviction request was already created, the update event handler will catch the second annotation addition and the cache gets synced. In the worst case the limit of max number of pods evicted gets exceeded.|The descheduler waits until all evacuation CRs are synced.|

Given the evacuation API is still a work in progress without any
existing implementation, the annotation based eviction can be seen
as an v1alpha1 implemenation of this proposal.
Integration with the evacuation API as an v1alphaN or beta implementation.

### Workflow Description

This section outlines the workflow for enabling the proposed evictions
in background that prevent workloads from being evicted right away
without following additional eviction policies.

#### Actors

- Cluster Administrator: Responsible for configuring descheduling
  policies and overseeing cluster operations.
- User: Individuals or services running workloads within the cluster.
- Descheduler: Component responsible for requesting pod evictions

#### Workflow Steps

##### Annotation based evacuation (v1alpha1)

1. Policy configuration
   1. The cluster administrator configures the descheduler to enable the new functionality.
2. A user deploys a workload through various nodes
   1. A scheduler distributes the pods based on configured scheduling policies
3. Eviction nomination
   1. At the beginning of each descheduling cycle the descheduler increases
      all the internal counters to take into account all pods that are subjects
      to background eviction. By checking `descheduler.alpha.kubernetes.io/eviction-in-progress`
      annotation or confronting the internal caches.
   2. Each descheduling plugin nominates a set of pods to be evicted.
   3. All the nominated pods are sorted based on the eviction-in-progress first priority.
4. Pod eviction: the descheduler starts evicting nominated pods until a limit is reached
   1. If a pod is not annotated with `descheduler.alpha.kubernetes.io/request-evict-only` evict
      the pod normally without additional handling.
   2. If a pod is annotated with `descheduler.alpha.kubernetes.io/request-evict-only` evict the pod.
      1. If the eviction fails intepret the error as a request for eviction in background.
      2. Error code, resp. response text is checked to distinguish between
         a genuine error and a confirmation of an eviction in background.

##### Evacuation API (v1alphaN or beta)

1. Policy configuration
   1. The cluster administrator configures the descheduler to enable the new functionality.
2. A user deploys a workload through various nodes
   1. A scheduler distributes the pods based on configured scheduling policies
3. Eviction nomination
   1. At the beginning of each descheduling cycle the descheduler increases
      all the internal counters to take into account all pods that are subjects
      to background eviction. By listing evacuation requests or confronting the internal caches.
   2. Each descheduling plugin nominates a set of pods to be evicted.
   3. All the nominated pods are sorted based on the eviction-in-progress first priority.
4. Pod eviction: the descheduler creates an evacuation request for each nominated pod until a limit is reached

### Notes/Constraints/Caveats (Optional)

- The upstream eviction API does not currently support evictions in background
  implemented by external components. For that, there's no community
  dedicated error code nor a response text for this type of eviction.
- The eviction in background error code will be temporary mapped to 429 (TooManyRequests).
- The response text will be searched for strings
  containing "Eviction triggered" prefix or similar.
- The descheduler will still treat all the pods with eviction in background as cattle.
  It will only prioritize these pods to be processed sooner.
  There may still be unnecessary/sub-optimal evictions.

### Risks and Mitigations

- If an external component responsible for eviction in background
  does not work properly the descheduler might evict less pods than expected.
  Leaving pods that require eviction (e.g. long running or violating third
  party policies) running longer than expected.
  The descheduler will need to implement a TTL mechanism to abandon
  eviction requests that take longer than expected.
- Most of the heavy lifting happens in the external components.
  The descheduler only acknowledges pods that are evicted in background.
  All the internal counters take these pods into account.
  When these pods are not properly evicted the internal counters might limit
  eviction of other pods that (when evicted) might improve overall cluster
  resource utilization.
- This new functionality needs to stay feature gated until
  the evacuation API is available and stable.
- When `descheduler.alpha.kubernetes.io/request-evict-only` annotation is deprecated
  workload that forgets to remove the annotation will not be evacuated.

## Design Details

### Implementation Strategies

#### API changes

- **New category of pods for eviction**: Two new annotations are introduced:
  - `descheduler.alpha.kubernetes.io/request-evict-only`: to signal pods whose
    eviction is not expected to be completed right away.
    Instead, an eviction request is expected to be intercepted by an external
    component which will initiate the eviction process for the pod.
    E.g. by performing live migration or scheduling the eviction to a more suitable time.
  - `descheduler.alpha.kubernetes.io/eviction-in-progress`: to mark pods whose
    eviction was initiated by an external component.
- **New eviction limits**: introduction of new `MaxNoOfPodsEvacuationsPerNode` and
  `MaxNoOfPodsEvacuationsPerNamespace` feature gated v1alpha1 fields.
- **New configurable list of eviction message prefixes (introduced later when needed)**:
  each third party validating admission webhook can provide different response message.
  It is not feasible to ask every party to change their messages for a temporary solution
  until the evacuation API is in place. Thus, temporary introducing a new feature
  gated `evictionMessagePrefixes` field (`[]string`) for a list of known prefixes
  signaling an eviction in background.
  Normally, "Eviction triggered" prefix will be acknowledged by default.

#### v1alpha1

- Keep track of evictions in progress: A new cache will be introduced
  to keep track of pods annotated with `descheduler.alpha.kubernetes.io/request-evict-only`
  that were requested to be evicted through the upstream eviction API.
  In case a descheduler gets unexpectedly restarted any pod annotated
  with `descheduler.alpha.kubernetes.io/eviction-in-progress` will be added
  to the cache if not already present.
  The cache will help with sorting and avoiding a duplicated eviction
  request of pods that were not annotated
  with `descheduler.alpha.kubernetes.io/eviction-in-progress` quickly enough.
  Each item in the cache will have a TTL attribute to address cases where an eviction
  request was intercepted by an external component but the component failed to annotate the pod.
- Implement a new built-in sorting plugin: Extend the descheduling framework
  with a new sorting plugin that will prefer pods
  with `descheduler.alpha.kubernetes.io/eviction-in-progress` annotation or those
  that are already in the cache of to-be-evicted pods.
  The plugin will be enabled when the new functionality is enabled (through a feature gate).
  Each plugin will have option to either take the new sorting into account or not.
  The sorting plugin can be executed either in pre-filter or pre-eviction phase.
- Implement a multi-level sorting: Allow plugins to first sort pods based
  on eviction-in-progress quality and then based on configured sorting plugins.
  Preferring pods that are already in a process of eviction over other pods
  is a crucial step for reducing unnecessary disruptions.
- Internal counters aware of eviction in progress: At the beginning of each
  descheduling loop all internal counters will take into account pods
  that are either annotated with `descheduler.alpha.kubernetes.io/eviction-in-progress`
  or are present in the cache of to-be-evicted pods.

#### v1alphaN or beta
- Replace the current eviction API with the new
  [evacuation API](https://github.com/kubernetes/enhancements/pull/4565) by default.
  Both `descheduler.alpha.kubernetes.io/request-evict-only` and
  `descheduler.alpha.kubernetes.io/eviction-in-progress` will be deprecated.
  Existing deployments will need to remove `descheduler.alpha.kubernetes.io/request-evict-only`
  annotation to utilize the new evacuation API.
  Otherwise, the eviction API will be used as a fallback.
  Workload owners utilizing the new evacuation API are expected to replace both annotations
  with [evacuation.coordination.k8s.io/priority_${EVACUATOR_CLASS}: ${PRIORITY}/${ROLE}](https://github.com/kubernetes/enhancements/blob/2d7dbab6c95575d68395c30fa789f87d9a2c3c01/keps/sig-apps/4563-evacuation-api/README.md#evacuator).
  Important: the new annotation have different semantics than the deprecated ones.
  Users are strongly recommended to review the evacuation API proposal for necessary details.
- The evacuation API expects the evacuation request to be reconciled during each descheduling cycle.
  Given each cycle starts from scratch the descheduler needs to first list
  all evacuation requests and sort the pods accordingly in each plugin.
  Other components may create an evacuation request as well.
  The descheduler needs to take this into account.
  E.g. by creating a snapshot (per cycle, per strategy) to avoid inconsistencies.
  An evacuation request may exist without a corresponding pod
  before the evacuation controller garbage collects the request.
- The descheduler needs to keep a cache of evacuation API requests (CRs)
  and remove itself from finalizers if the request is no longer needed.
  E.g. `evacuation.coordination.k8s.io/instigator_descheduler.sigs.k8s.io` as a finalizer.
  The removal is allowed only when the evacuation request has its cancellation policy set `Allow`.
- Read the eviction status from the [evacuation CR status](https://github.com/kubernetes/enhancements/pull/4565).

#### Code changes

The code responsible for invoking the eviction API is located
under `sigs.k8s.io/descheduler/pkg/descheduler/evictions` under private `evictPod` function.
The function can be generalized into a generic interface:

```go
type PodEvacuator interface {
    EvacutePod(ctx context.Context, pod *v1.Pod) error
}
```

to provide the [descheduling framework](https://github.com/kubernetes-sigs/descheduler/pull/1372)
a generic way of evicting/evacuating pods.
The new interface can be used for implementing various testing scenarios.
Or turned into a plugin if needed.

#### Metrics

A new metric `pod_evacuations` for counting the number of evacuated pods is introduced.
The descheduler will not observe whether a pod was ultimately evicted.
Only provide a summary about how many pods were requested to be evacuated.
If needed, another metric `pod_evacuations_in_progress` for counting
the number of evacuations that are reconciled can be introduced.

### Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.

* The evacuation API proposal mentions the evacuation instigator should remove
  its intent when the evacuation is no longer necessary.
  Should each descheduling cycle reset all the eviction requests
  or wait until some of the eviction requests were completed
  to free a "bucket" for new evictions?
  **Answer**: The first implementation will not reset any evacuation request.
  The descheduler will account for existing requests and update the internal counters accordingally.
  In the future a mechanism for deleting too old evacuation requests can be introduced.
  I.e. based on a new `--max-evacuation-request-ttl` option.
* The evacuation API proposal mentions more than a one entity can request an eviction.
  Should the descheduler take these into account as well?
  What if another entity decides to evict a pod that is of a low priority
  from the descheduler's point of view?
  **Answer**: The first implementation will consider all pods with an existing
  evacuation request as "already getting evacuated". Independent of the pod priorities.
* With evacuation requests a plugin does not evict a pod right away.
  Meaning, other plugins will need to check whether a pod has a corresponding
  evacuation request present.
  If so, take each such pod into account when constructing a list of nominated pods.
  For that it might be easier to see the current evictions as marking pods
  for future evacuation and perform the actual eviction/evacuation
  at the end of each descheduling cycle instead of evicting pods from within plugins.
  **Answer**: This is a breaking change for existing plugins.
  The first implementation will filter out pods that has already
  an existing evacuation request present.
  Keeping the backward compatibility with the current implementation.
  Later, a new proposal can be created to discuss the possibility
  of evicting pods at the end of each descheduling cycle (after
  both `Deschedule` and `Balance` extension points of all plugins were invoked).

### Test Plan

<!--
**Note:** *Not required until targeted at a release.*
The goal is to ensure that we don't accept enhancements with inadequate testing.

All code is expected to have adequate tests (eventually with coverage
expectations). Please adhere to the [Kubernetes testing guidelines][testing-guidelines]
when drafting this test plan.

[testing-guidelines]: https://git.k8s.io/community/contributors/devel/sig-testing/testing.md
-->

[x] I/we understand the owners of the involved components may require updates to
existing tests to make this code solid enough prior to committing the changes necessary
to implement this enhancement.

##### Prerequisite testing updates

<!--
Based on reviewers feedback describe what additional tests need to be added prior
implementing this enhancement to ensure the enhancements have also solid foundations.
-->

##### Unit tests

<!--
In principle every added code should have complete unit test coverage, so providing
the exact set of tests will not bring additional value.
However, if complete unit test coverage is not possible, explain the reason of it
together with explanation why this is acceptable.
-->

<!--
Additionally, for Alpha try to enumerate the core package you will be touching
to implement this enhancement and provide the current unit coverage for those
in the form of:
- <package>: <date> - <current test coverage>
The data can be easily read from:
https://testgrid.k8s.io/sig-testing-canaries#ci-kubernetes-coverage-unit

This can inform certain test coverage improvements that we want to do before
extending the production code to implement this enhancement.
-->

Current coverages:
- `sigs.k8s.io/descheduler/pkg/descheduler/evictions`: `2024-05-13` - `4.3`
- `sigs.k8s.io/descheduler/pkg/framework/plugins/nodeutilization`: `2024-05-13` - `48.7`
- `sigs.k8s.io/descheduler/pkg/framework/plugins/podlifetime`: `2024-05-13` - `18.2`
- `sigs.k8s.io/descheduler/pkg/framework/plugins/removeduplicates`: `2024-05-13` - `32.3`
- `sigs.k8s.io/descheduler/pkg/framework/plugins/removefailedpods`: `2024-05-13` - `21.6`
- `sigs.k8s.io/descheduler/pkg/framework/plugins/removepodshavingtoomanyrestarts`: `2024-05-13` - `22.0`
- `sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatinginterpodantiaffinity`: `2024-05-13` - `17.4`
- `sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodeaffinity`: `2024-05-13` - `18.0`
- `sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingnodetaints`: `2024-05-13` - `18.2`
- `sigs.k8s.io/descheduler/pkg/framework/plugins/removepodsviolatingtopologyspreadconstraint`: `2024-05-13` - `43.0`

These tests will be added:
- New tests  will be added under `sigs.k8s.io/descheduler/pkg/framework/plugins` for each
  mentioned plugin to test evacuation (replacement of the eviction API).
- New tests will be added under `sigs.k8s.io/descheduler/pkg/descheduler/evictions` to test
  the new evacuation (annotation based, API based) implementation.
- New tests under  `sigs.k8s.io/descheduler/pkg/descheduler` will be created for:
  - the new internal sorting and filtering algorithms
  - validating the evacuation limits are respected with already existing
    evacuation requests from a previous descheduling cycle

##### Integration tests

<!--
Integration tests are contained in k8s.io/kubernetes/test/integration.
Integration tests allow control of the configuration parameters used to start the binaries under test.
This is different from e2e tests which do not allow configuration of parameters.
Doing this allows testing non-default options and multiple different and potentially conflicting command line options.
-->

<!--
This question should be filled when targeting a release.
For Alpha, describe what tests will be added to ensure proper quality of the enhancement.

For Beta and GA, add links to added tests together with links to k8s-triage for those tests:
https://storage.googleapis.com/k8s-triage/index.html
-->

No integration tests available

##### e2e tests

<!--
This question should be filled when targeting a release.
For Alpha, describe what tests will be added to ensure proper quality of the enhancement.

For Beta and GA, add links to added tests together with links to k8s-triage for those tests:
https://storage.googleapis.com/k8s-triage/index.html

We expect no non-infra related flakes in the last month as a GA graduation criteria.
-->

These tests will be added:
- `EvacuationInBackground` for testing an evacuation request/annotated pod eviction:
  1. Setup a validating admission webhook that will intercept an eviction request,
     start a pod eviction/deletion in e.g. T=5s but rejects the eviction request.
  2. Create an evacuation request for a pod/evict a pod
     with `descheduler.alpha.kubernetes.io/request-evict-only` annotation.
  3. Observe the pod is not removed right away and wait for the configured
     delay T before checking a pod is removed.
  4. Check a pod was removed after T, not before.
- `EvacuationLimitReached` for testing only a limited amount of pods is evacuated
  with already existing evacuation requests:
  1. Create evacuation requests (e.g. by annotating pods
     with `descheduler.alpha.kubernetes.io/eviction-in-progress` or creating evacuation requests).
  2. Run the descheduler and observe only an expected amount of pods
     is requested to be evicted/evacuated.
- `EvacuationTTL` for testing an evacuation request (with a single owner
  in case of the evacuation API) is canceled after a configured TTL:
  1. Setup a validating admission webhook that will intercept an eviction request,
     start a pod eviction initiation but rejects the eviction request.
  2. Wait for e.g. 10s (TTL).
  3. At the beginning of the next descheduling cycle the descheduler checks for all
     evacuation requests/pods with `descheduler.alpha.kubernetes.io/request-evict-only` annotation
     and remove finalizer/entry from the cache.
  4. During the descheduling cycle the descheduler creates eviction/evacuation requests.
  5. Check there's an expected number of eviction/evacuation requests.

### Graduation Criteria

<!--
**Note:** *Not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, [feature gate] graduations, or as
something else. The KEP should keep this high-level with a focus on what
signals will be looked at to determine graduation.

Consider the following in developing the graduation criteria for this enhancement:
- [Maturity levels (`alpha`, `beta`, `stable`)][maturity-levels]
- [Feature gate][feature gate] lifecycle
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc
definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning)
or by redefining what graduation means.

In general we try to use the same stages (alpha, beta, GA), regardless of how the
functionality is accessed.

[feature gate]: https://git.k8s.io/community/contributors/devel/sig-architecture/feature-gates.md
[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

Below are some examples to consider, in addition to the aforementioned [maturity levels][maturity-levels].

#### Alpha

- Feature implemented behind a feature flag
- Initial e2e tests completed and enabled

#### Beta

- Gather feedback from developers and surveys
- Complete features A, B, C
- Additional tests are in Testgrid and linked in KEP

#### GA

- N examples of real-world usage
- N installs
- More rigorous forms of testing—e.g., downgrade tests and scalability tests
- Allowing time for feedback

**Note:** Generally we also wait at least two releases between beta and
GA/stable, because there's no opportunity for user feedback, or even bug reports,
in back-to-back releases.

**For non-optional features moving to GA, the graduation criteria must include
[conformance tests].**

[conformance tests]: https://git.k8s.io/community/contributors/devel/sig-architecture/conformance-tests.md

#### Deprecation

- Announce deprecation and support policy of the existing flag
- Two versions passed since introducing the functionality that deprecates the flag (to address version skew)
- Address feedback on usage/changed behavior, provided on GitHub issues
- Deprecate the flag
-->

#### Alpha

- Feature implemented behind a feature flag
- Initial e2e tests completed and enabled
- Annotation based evictions
- Workloads do not set `descheduler.alpha.kubernetes.io/request-evict-only` annotation
  when deprecated

#### Beta

- Gather feedback from developers and surveys
- Evacuation API based evictions
- Identifying corner cases that are not addressed by the evacuation API
- `descheduler.alpha.kubernetes.io/request-evict-only` annotation is deprecated
- Workloads do not set `descheduler.alpha.kubernetes.io/request-evict-only` annotation

#### GA

- At least 3 examples of real-world usage
- Many adoptions of the new feature
- No negative feedback
- No bug issues reported

### Upgrade / Downgrade Strategy

<!--
If applicable, how will the component be upgraded and downgraded? Make sure
this is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade, in order to maintain previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade, in order to make use of the enhancement?
-->

### Version Skew Strategy

<!--
If applicable, how will the component handle version skew with other
components? What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- Does this enhancement involve coordinating behavior in the control plane and nodes?
- How does an n-3 kubelet or kube-proxy without this feature available behave when this feature is used?
- How does an n-1 kube-controller-manager or kube-scheduler without this feature available behave when this feature is used?
- Will any other components on the node change? For example, changes to CSI,
  CRI or CNI may require updating that component before the kubelet.
-->

## Production Readiness Review Questionnaire

<!--

Production readiness reviews are intended to ensure that features merging into
Kubernetes are observable, scalable and supportable; can be safely operated in
production environments, and can be disabled or rolled back in the event they
cause increased failures in production. See more in the PRR KEP at
https://git.k8s.io/enhancements/keps/sig-architecture/1194-prod-readiness.

The production readiness review questionnaire must be completed and approved
for the KEP to move to `implementable` status and be included in the release.

In some cases, the questions below should also have answers in `kep.yaml`. This
is to enable automation to verify the presence of the review, and to reduce review
burden and latency.

The KEP must have a approver from the
[`prod-readiness-approvers`](http://git.k8s.io/enhancements/OWNERS_ALIASES)
team. Please reach out on the
[#prod-readiness](https://kubernetes.slack.com/archives/CPNHUMN74) channel if
you need any help or guidance.
-->

### Feature Enablement and Rollback

<!--
This section must be completed when targeting alpha to a release.
-->

###### How can this feature be enabled / disabled in a live cluster?

<!--
Pick one of these and delete the rest.

Documentation is available on [feature gate lifecycle] and expectations, as
well as the [existing list] of feature gates.

[feature gate lifecycle]: https://git.k8s.io/community/contributors/devel/sig-architecture/feature-gates.md
[existing list]: https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/
-->

- [ ] Feature gate (also fill in values in `kep.yaml`)
  - Feature gate name:
  - Components depending on the feature gate:
- [ ] Other
  - Describe the mechanism:
  - Will enabling / disabling the feature require downtime of the control
    plane?
  - Will enabling / disabling the feature require downtime or reprovisioning
    of a node?

###### Does enabling the feature change any default behavior?

<!--
Any change of default behavior may be surprising to users or break existing
automations, so be extremely careful here.
-->

Eviction of pods can be delayed. Users need to make sure they have a reliable external
components that can evict a pod based on the corresponding evacuation request.

For the annotation based eviction only pods annotated
with `descheduler.alpha.kubernetes.io/request-evict-only` are effected.
The eviction itself is still performed through the eviction API.
Only the error (code) is interpreted differently.
Pods with both annotations present are not evicted more than once.

###### Can the feature be disabled once it has been enabled (i.e. can we roll back the enablement)?

<!--
Describe the consequences on existing workloads (e.g., if this is a runtime
feature, can it break the existing applications?).

Feature gates are typically disabled by setting the flag to `false` and
restarting the component. No other changes should be necessary to disable the
feature.

NOTE: Also set `disable-supported` to `true` or `false` in `kep.yaml`.
-->

Yes.

###### What happens if we reenable the feature if it was previously rolled back?

The same as when the feature is enabled the first time.

###### Are there any tests for feature enablement/disablement?

<!--
The e2e framework does not currently support enabling or disabling feature
gates. However, unit tests in each component dealing with managing data, created
with and without the feature, are necessary. At the very least, think about
conversion tests if API types are being modified.

Additionally, for features that are introducing a new API field, unit tests that
are exercising the `switch` of feature gate itself (what happens if I disable a
feature gate after having objects written with the new field) are also critical.
You can take a look at one potential example of such test in:
https://github.com/kubernetes/kubernetes/pull/97058/files#diff-7826f7adbc1996a05ab52e3f5f02429e94b68ce6bce0dc534d1be636154fded3R246-R282
-->

### Rollout, Upgrade and Rollback Planning

<!--
This section must be completed when targeting beta to a release.
-->

###### How can a rollout or rollback fail? Can it impact already running workloads?

<!--
Try to be as paranoid as possible - e.g., what if some components will restart
mid-rollout?

Be sure to consider highly-available clusters, where, for example,
feature flags will be enabled on some API servers and not others during the
rollout. Similarly, consider large clusters and how enablement/disablement
will rollout across nodes.
-->

###### What specific metrics should inform a rollback?

<!--
What signals should users be paying attention to when the feature is young
that might indicate a serious problem?
-->

###### Were upgrade and rollback tested? Was the upgrade->downgrade->upgrade path tested?

<!--
Describe manual testing that was done and the outcomes.
Longer term, we may want to require automated upgrade/rollback tests, but we
are missing a bunch of machinery and tooling and can't do that now.
-->

###### Is the rollout accompanied by any deprecations and/or removals of features, APIs, fields of API types, flags, etc.?

<!--
Even if applying deprecation policies, they may still surprise some users.
-->

### Monitoring Requirements

<!--
This section must be completed when targeting beta to a release.

For GA, this section is required: approvers should be able to confirm the
previous answers based on experience in the field.
-->

###### How can an operator determine if the feature is in use by workloads?

<!--
Ideally, this should be a metric. Operations against the Kubernetes API (e.g.,
checking if there are objects with field X set) may be a last resort. Avoid
logs or events for this purpose.
-->

###### How can someone using this feature know that it is working for their instance?

<!--
For instance, if this is a pod-related feature, it should be possible to determine if the feature is functioning properly
for each individual pod.
Pick one more of these and delete the rest.
Please describe all items visible to end users below with sufficient detail so that they can verify correct enablement
and operation of this feature.
Recall that end users cannot usually observe component logs or access metrics.
-->

- [ ] Events
  - Event Reason:
- [ ] API .status
  - Condition name:
  - Other field:
- [ ] Other (treat as last resort)
  - Details:

###### What are the reasonable SLOs (Service Level Objectives) for the enhancement?

<!--
This is your opportunity to define what "normal" quality of service looks like
for a feature.

It's impossible to provide comprehensive guidance, but at the very
high level (needs more precise definitions) those may be things like:
  - per-day percentage of API calls finishing with 5XX errors <= 1%
  - 99% percentile over day of absolute value from (job creation time minus expected
    job creation time) for cron job <= 10%
  - 99.9% of /health requests per day finish with 200 code

These goals will help you determine what you need to measure (SLIs) in the next
question.
-->

###### What are the SLIs (Service Level Indicators) an operator can use to determine the health of the service?

<!--
Pick one more of these and delete the rest.
-->

- [ ] Metrics
  - Metric name:
  - [Optional] Aggregation method:
  - Components exposing the metric:
- [ ] Other (treat as last resort)
  - Details:

###### Are there any missing metrics that would be useful to have to improve observability of this feature?

<!--
Describe the metrics themselves and the reasons why they weren't added (e.g., cost,
implementation difficulties, etc.).
-->

### Dependencies

<!--
This section must be completed when targeting beta to a release.
-->

###### Does this feature depend on any specific services running in the cluster?

<!--
Think about both cluster-level services (e.g. metrics-server) as well
as node-level agents (e.g. specific version of CRI). Focus on external or
optional services that are needed. For example, if this feature depends on
a cloud provider API, or upon an external software-defined storage or network
control plane.

For each of these, fill in the following—thinking about running existing user workloads
and creating new ones, as well as about cluster-level services (e.g. DNS):
  - [Dependency name]
    - Usage description:
      - Impact of its outage on the feature:
      - Impact of its degraded performance or high-error rates on the feature:
-->

### Scalability

<!--
For alpha, this section is encouraged: reviewers should consider these questions
and attempt to answer them.

For beta, this section is required: reviewers must answer these questions.

For GA, this section is required: approvers should be able to confirm the
previous answers based on experience in the field.
-->

###### Will enabling / using this feature result in any new API calls?

<!--
Describe them, providing:
  - API call type (e.g. PATCH pods)
  - estimated throughput
  - originating component(s) (e.g. Kubelet, Feature-X-controller)
Focusing mostly on:
  - components listing and/or watching resources they didn't before
  - API calls that may be triggered by changes of some Kubernetes resources
    (e.g. update of object X triggers new updates of object Y)
  - periodic API calls to reconcile state (e.g. periodic fetching state,
    heartbeats, leader election, etc.)
-->

When the evacuation API is used new evacuation create/update/list/watch requests are sent.

###### Will enabling / using this feature result in introducing new API types?

<!--
Describe them, providing:
  - API type
  - Supported number of objects per cluster
  - Supported number of objects per namespace (for namespace-scoped objects)
-->

No.

###### Will enabling / using this feature result in any new calls to the cloud provider?

<!--
Describe them, providing:
  - Which API(s):
  - Estimated increase:
-->

No.

###### Will enabling / using this feature result in increasing size or count of the existing API objects?

<!--
Describe them, providing:
  - API type(s):
  - Estimated increase in size: (e.g., new annotation of size 32B)
  - Estimated amount of new objects: (e.g., new Object X for every existing Pod)
-->

No.

###### Will enabling / using this feature result in increasing time taken by any operations covered by existing SLIs/SLOs?

<!--
Look at the [existing SLIs/SLOs].

Think about adding additional work or introducing new steps in between
(e.g. need to do X to start a container), etc. Please describe the details.

[existing SLIs/SLOs]: https://git.k8s.io/community/sig-scalability/slos/slos.md#kubernetes-slisslos
-->

Very likely. When the evacuation API is used the eviction itself very depends on existing external controllers facilitating the eviction.

###### Will enabling / using this feature result in non-negligible increase of resource usage (CPU, RAM, disk, IO, ...) in any components?

<!--
Things to keep in mind include: additional in-memory state, additional
non-trivial computations, excessive access to disks (including increased log
volume), significant amount of data sent and/or received over network, etc.
This through this both in small and large cases, again with respect to the
[supported limits].

[supported limits]: https://git.k8s.io/community//sig-scalability/configs-and-limits/thresholds.md
-->

No.

###### Can enabling / using this feature result in resource exhaustion of some node resources (PIDs, sockets, inodes, etc.)?

<!--
Focus not just on happy cases, but primarily on more pathological cases
(e.g. probes taking a minute instead of milliseconds, failed pods consuming resources, etc.).
If any of the resources can be exhausted, how this is mitigated with the existing limits
(e.g. pods per node) or new limits added by this KEP?

Are there any tests that were run/should be run to understand performance characteristics better
and validate the declared limits?
-->

### Troubleshooting

<!--
This section must be completed when targeting beta to a release.

For GA, this section is required: approvers should be able to confirm the
previous answers based on experience in the field.

The Troubleshooting section currently serves the `Playbook` role. We may consider
splitting it into a dedicated `Playbook` document (potentially with some monitoring
details). For now, we leave it here.
-->

###### How does this feature react if the API server and/or etcd is unavailable?

###### What are other known failure modes?

<!--
For each of them, fill in the following information by copying the below template:
  - [Failure mode brief description]
    - Detection: How can it be detected via metrics? Stated another way:
      how can an operator troubleshoot without logging into a master or worker node?
    - Mitigations: What can be done to stop the bleeding, especially for already
      running user workloads?
    - Diagnostics: What are the useful log messages and their required logging
      levels that could help debug the issue?
      Not required until feature graduated to beta.
    - Testing: Are there any tests for failure mode? If not, describe why.
-->

###### What steps should be taken if SLOs are not being met to determine the problem?

## Implementation History

<!--
Major milestones in the lifecycle of a KEP should be tracked in this section.
Major milestones might include:
- the `Summary` and `Motivation` sections being merged, signaling SIG acceptance
- the `Proposal` section being merged, signaling agreement on a proposed design
- the date implementation started
- the first Kubernetes release where an initial version of the KEP was available
- the version of Kubernetes where the KEP graduated to general availability
- when the KEP was retired or superseded
-->

- 2024-04-14: Initial draft KEP

## Drawbacks

<!--
Why should this KEP _not_ be implemented?
-->

## Alternatives

<!--
What other approaches did you consider, and why did you rule them out? These do
not need to be as detailed as the proposal, but should include enough
information to express the idea and why it was not acceptable.
-->

## Infrastructure Needed (Optional)

<!--
Use this section if you need things from the project/SIG. Examples include a
new subproject, repos requested, or GitHub details. Listing these here allows a
SIG to get the process for these resources started right away.
-->
