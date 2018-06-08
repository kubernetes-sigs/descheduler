#debuginfo not supported with Go
%global debug_package %{nil}
# modifying the Go binaries breaks the DWARF debugging
%global __os_install_post %{_rpmconfigdir}/brp-compress

%global gopath      %{_datadir}/gocode
%global import_path github.com/openshift/descheduler

%global golang_version 1.9.1
%{!?version: %global version 0.4.0}
%{!?release: %global release 1}

%{!?commit: %global commit HEAD }
%global shortcommit %(c=%{commit}; echo ${c:0:7})
# os_git_vars needed to run hack scripts during rpm builds
%{!?os_git_vars: %global os_git_vars OS_GIT_VERSION='' OS_GIT_COMMIT='' OS_GIT_MAJOR='' OS_GIT_MINOR='' OS_GIT_TREE_STATE='' }

%if 0%{?skip_build}
%global do_build 0
%else
%global do_build 1
%endif
%if 0%{?skip_prep}
%global do_prep 0
%else
%global do_prep 1
%endif

%if "%{dist}" == ".el7aos"
%global package_name atomic-openshift
%global product_name Atomic OpenShift
%else
%global package_name origin
%global product_name Origin
%endif

Name:           atomic-openshift-descheduler
Version:        %{version}
Release:        %{release}%{?dist}
Summary:        Descheduler for OpenShift and Kubernetes
License:        ASL 2.0
URL:            https://%{import_path}

Source0:        https://%{import_path}/archive/%{commit}/%{name}-%{version}.tar.gz
BuildRequires:  golang >= %{golang_version}

%description
OpenShift is a distribution of Kubernetes optimized for enterprise application
development and deployment. Descheduler is a tool that analyzes a cluster, and
evicts some running pods based on its policy, so that the evicted pods can be
rescheduled on more suitable nodes. 

%prep
%if 0%{do_prep}
%setup -q
%endif

%build
%if 0%{do_build}
%ifarch x86_64
  BUILD_PLATFORM="linux/amd64"
%endif
%ifarch %{ix86}
  BUILD_PLATFORM="linux/386"
%endif
%ifarch ppc64le
  BUILD_PLATFORM="linux/ppc64le"
%endif
%ifarch %{arm} aarch64
  BUILD_PLATFORM="linux/arm64"
%endif
%ifarch s390x
  BUILD_PLATFORM="linux/s390x"
%endif
OS_ONLY_BUILD_PLATFORMS="${BUILD_PLATFORM}" %{os_git_vars} make build-cross
%endif

%install
PLATFORM="$(go env GOHOSTOS)/$(go env GOHOSTARCH)"
install -d %{buildroot}%{_bindir}

# Install linux components
for bin in descheduler
do
  echo "+++ INSTALLING ${bin}"
  install -p -m 755 _output/local/bin/${PLATFORM}/${bin} %{buildroot}%{_bindir}/${bin}
done

%files
%doc README.md
%license LICENSE
%doc examples/*
%{_bindir}/descheduler

%changelog
* Tue Jan 9 2018 Avesh Agarwal <avagarwa@redhat.com> 0.4.0-1
- Update to 0.4.0 (rebase to kube 1.9)

* Mon Nov 13 2017 Avesh Agarwal <avagarwa@redhat.com> 0.3.0-1
- Update to 0.3.0

* Tue Oct 17 2017 Avesh Agarwal <avagarwa@redhat.com> 0.2.0-1
- Update to 0.2.0

* Thu Oct 5 2017 Avesh Agarwal <avagarwa@redhat.com> 0.1.0-1
- Initial package version
