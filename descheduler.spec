#debuginfo not supported with Go
%global debug_package %{nil}
# modifying the Go binaries breaks the DWARF debugging
%global __os_install_post %{_rpmconfigdir}/brp-compress

%global gopath      %{_datadir}/gocode
%global import_path github.com/openshift/descheduler

%global golang_version 1.8.3

%if "%{dist}" == ".el7aos"
%global package_name atomic-openshift
%global product_name Atomic OpenShift
%else
%global package_name origin
%global product_name Origin
%endif

Name:           atomic-openshift-descheduler
Version:        0.4.0
Release:        1%{?dist}
Summary:        Descheduler for OpenShift and Kubernetes
License:        ASL 2.0
URL:            https://%{import_path}

Source0:        v0.4.0.tar.gz
BuildRequires:  golang >= %{golang_version}

%description
OpenShift is a distribution of Kubernetes optimized for enterprise application
development and deployment. Descheduler is a tool that analyzes a cluster, and
evicts some running pods based on its policy, so that the evicted pods can be
rescheduled on more suitable nodes. 

%prep
%setup -n descheduler-%{version}

%build
export GOPATH=`pwd`/go
mkdir `pwd`/../descheduler
mv * `pwd`/../descheduler
mkdir -p `pwd`/go/src/github.com/kubernetes-incubator
mv `pwd`/../descheduler `pwd`/go/src/github.com/kubernetes-incubator
cd go/src/github.com/kubernetes-incubator/descheduler
make

%install
install -d %{buildroot}%{_bindir}
install -p -m 755 go/src/github.com/kubernetes-incubator/descheduler/_output/bin/descheduler %{buildroot}%{_bindir}/descheduler

%files
%doc go/src/github.com/kubernetes-incubator/descheduler/README.md
%doc go/src/github.com/kubernetes-incubator/descheduler/examples/*
%license go/src/github.com/kubernetes-incubator/descheduler/LICENSE
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
