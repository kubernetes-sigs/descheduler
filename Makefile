all: build
.PHONY: all

GO=GO111MODULE=on GOFLAGS=-mod=vendor go

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/images.mk \
)

# Exclude e2e tests from unit testing
GO_TEST_PACKAGES :=./pkg/... ./cmd/...
GO_BUILD_FLAGS :=-tags strictfipsruntime
IMAGE_REGISTRY :=registry.svc.ci.openshift.org

# This will call a macro called "build-image" which will generate image specific targets based on the parameters:
# $0 - macro name
# $1 - target name
# $2 - image ref
# $3 - Dockerfile path
# $4 - context directory for image build
$(call build-image,descheduler,$(CI_IMAGE_REGISTRY)/ocp/4.12:descheduler,./images/ci/Dockerfile,.)

test-e2e: GO_TEST_PACKAGES :=./test/e2e
test-e2e: GO_TEST_FLAGS :=-v
test-e2e: test-unit
.PHONY: test-e2e

clean:
	$(RM) -r ./apiserver.local.config
	$(RM) -r ./_output
.PHONY: clean
