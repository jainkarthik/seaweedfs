Name:           seaweedfs
Version:        4.12
Release:        1%{?dist}
Summary:        SeaweedFS Distributed Object Store and File System
License:        Apache-2.0
URL:            https://github.com/seaweedfs/seaweedfs
BuildArch:      noarch

Requires:       shadow-utils

%description
SeaweedFS is a distributed object storage and file system designed for
large scale data storage. It features:
- Distributed hash-based file storage
- S3-compatible API
- Filer for POSIX-like access
- Volume server for actual storage
- Master server for cluster management

%prep
# Nothing to prep - binary is built externally

%build
# Nothing to build - binary is built externally

%install
# Create directories
mkdir -p %{buildroot}/opt/weed
mkdir -p %{buildroot}/%{_sysconfdir}/default/weed
mkdir -p %{buildroot}/%{_sysconfdir}/weed
mkdir -p %{buildroot}/%{_localstatedir}/log/weed
mkdir -p %{buildroot}/%{_localstatedir}/lib/weed
mkdir -p %{buildroot}/%{_run}/weed
mkdir -p %{buildroot}/%{_libdir}/systemd/system
mkdir -p %{buildroot}/%{_sysconfdir}/logrotate.d
mkdir -p %{buildroot}/%{_sysconfdir}/haproxy/conf.d
mkdir -p %{buildroot}/opt/weed/bin

# Install binary
install -m 755 %{_sourcedir}/weed-%{version}-linux-amd64 %{buildroot}/opt/weed/weed

# Install systemd units
install -m 644 %{_sourcedir}/weed-master@.service %{buildroot}/%{_libdir}/systemd/system/
install -m 644 %{_sourcedir}/weed-volume@.service %{buildroot}/%{_libdir}/systemd/system/
install -m 644 %{_sourcedir}/weed-filer@.service %{buildroot}/%{_libdir}/systemd/system/
install -m 644 %{_sourcedir}/weed-s3@.service %{buildroot}/%{_libdir}/systemd/system/

# Install config
install -m 640 %{_sourcedir}/weed.conf %{buildroot}/%{_sysconfdir}/default/weed/weed.conf

# Install logrotate
install -m 644 %{_sourcedir}/logrotate.d/weed %{buildroot}/%{_sysconfdir}/logrotate.d/weed

# Install HAProxy config
install -m 644 %{_sourcedir}/haproxy-weed-s3.cfg %{buildroot}/%{_sysconfdir}/haproxy/conf.d/weed-s3.cfg

# Install healthcheck script
install -m 755 %{_sourcedir}/healthcheck.sh %{buildroot}/opt/weed/healthcheck.sh

%pre
# Create weed user if it doesn't exist
getent group weed >/dev/null || groupadd -r weed
getent passwd weed >/dev/null || \
    useradd -r -g weed -d /opt/weed -s /sbin/nologin \
    -c "SeaweedFS Service Account" weed
exit 0

%post
# Create required directories
mkdir -p /opt/weed
mkdir -p /var/log/weed
mkdir -p /var/lib/weed
mkdir -p /run/weed

# Set ownership
chown -R weed:weed /opt/weed /var/log/weed /var/lib/weed /run/weed 2>/dev/null || true

# Ensure binary is executable
if [ -f /opt/weed/weed ]; then
    chmod 0755 /opt/weed/weed
fi

# Reload systemd
systemctl daemon-reload 2>/dev/null || true

%preun
# Only run on final removal, not upgrades
if [ $1 -eq 0 ]; then
    # Stop all services
    systemctl stop 'weed-*@*' 2>/dev/null || true
    systemctl disable 'weed-*@*' 2>/dev/null || true
fi

%postun
# Only run on final removal, not upgrades
if [ $1 -eq 0 ]; then
    # Reload systemd
    systemctl daemon-reload 2>/dev/null || true

    # Remove user and group only on purge (handled by RPM's --purge)
    # User will be removed automatically by RPM if package is erased
fi

%files
%defattr(-,root,root,-)
/opt/weed/weed
/opt/weed/healthcheck.sh
%{_sysconfdir}/default/weed/weed.conf
%{_sysconfdir}/weed/
%{_libdir}/systemd/system/weed-master@.service
%{_libdir}/systemd/system/weed-volume@.service
%{_libdir}/systemd/system/weed-filer@.service
%{_libdir}/systemd/system/weed-s3@.service
%{_sysconfdir}/logrotate.d/weed
%{_sysconfdir}/haproxy/conf.d/weed-s3.cfg

%changelog
* Mon Feb 15 2026 SeaweedFS Team <team@seaweedfs.com> - 4.12-1
- Initial RPM package for SeaweedFS
- STIG-compliant systemd service templates
- Multi-instance support via systemd templating
