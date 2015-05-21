%define version    2.0.1
%define debug_package %{nil}
Name: docker-distribution
Version: %{version}
Release:  1%{?dist}
Summary: Docker Distribution server v2

Group: Container Management
License: Apache License
URL: https://github.com/docker/distribution
Source0: https://github.com/docker/distribution/archive/v%{version}.tar.gz

BuildRequires: make
Requires: nginx

%description
Docker Distribution server

%prep
%setup -q -n distribution-%{version}


%build
DISTRIBUTION_DIR=%{_topdir}/BUILD/distribution-%{version}
GOPATH=$DISTRIBUTION_DIR/Godeps/_workspace:%{_topdir}/BUILD
export GOPATH DISTRIBUTION_DIR
make PREFIX=%{_topdir}/BUILD/distribution-%{version}/usr clean binaries

%install
mkdir -p %{buildroot}/usr/bin
cp -p ./usr/bin/registry %{buildroot}/usr/bin/registry
cp -p ./usr/bin/dist %{buildroot}/usr/bin/dist
cp -p ./usr/bin/registry-api-descriptor-template %{buildroot}/usr/bin/registry-api-descriptor-template

%files
%doc
/usr/bin/registry
/usr/bin/dist
/usr/bin/registry-api-descriptor-template


%changelog



