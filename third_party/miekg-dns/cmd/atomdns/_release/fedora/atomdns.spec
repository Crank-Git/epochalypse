Name:	atomdns
Version:	086
Release:        1.0
Summary:        DNS server that chains handlers
License:        MIT
URL:            https://atomdns.miek.nl
Source0:        %{name}-%{version}.tar.gz
Source1:        atomdns.service
BuildRequires:  pkgconfig(systemd)
%{?systemd_ordering}

%description
DNS server that chains handlers.
https://atomdns.miek.nl

%define services atomdns.service
%define _topdir %(echo $PWD)/
%define _srcdir %{_topdir}/../../

%prep

%build
cd %{_srcdir}
CGO_ENABLED=0 go build

%install
cp %{_topdir}%{name}.service %{buildroot}/../%{name}.service
cp %{_topdir}%{name}.conf    %{buildroot}/../%{name}.conf
cp %{_srcdir}/%{name}        %{buildroot}/../%{name}
cp %{_topdir}Conffile        %{buildroot}/../Conffile

install -D -m 0755 %{name}            %{buildroot}/%{_bindir}/%{name}
install -D -m 0640 %{name}.conf       %{buildroot}/usr/lib/sysusers.d/%{name}.conf
install -D -m 0644 %{name}.service    %{buildroot}/%{_unitdir}/%{name}.service
install -D -m 0644 Conffile           %{buildroot}%{_sysconfdir}/%{name}/Conffile

install -d -m 0755 %{buildroot}%{_mandir}/man{1,7}
install -D -m 0644 %{_srcdir}man/*.1  %{buildroot}%{_mandir}/man1
install -D -m 0644 %{_srcdir}man/*.7  %{buildroot}%{_mandir}/man7

%files
%{_bindir}/%{name}
%{_unitdir}/%{name}.service
/usr/lib/sysusers.d/%{name}.conf
%{_mandir}/man{1,7}/*
%config(noreplace) %{_sysconfdir}/%{name}/Conffile

%post
%systemd_post atomdns.service
mkdir -p /var/lib/atomdns && chown atomdns:atomdns /var/lib/atomdns

%preun
%systemd_preun atomdns.service

%postun
%systemd_postun_with_restart atomdns.service
