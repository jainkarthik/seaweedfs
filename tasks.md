# Tasks for SeaweedFS Enterprise Build & Packaging

## Phase 1: Build System
- [x] 1.1 Create Makefile with multi-arch build targets (linux/amd64, linux/arm64)
- [x] 1.2 Implement FIPS build flag support (FIPS_ENABLED=true)
- [x] 1.3 Add version extraction from Git tags
- [x] 1.4 Configure reproducible build flags
- [x] 1.5 Add CGO and Go toolchain configuration

## Phase 2: Security Scanning
- [x] 2.1 Add syft SBOM generation (CycloneDX JSON + SPDX JSON)
- [x] 2.2 Add trivy vulnerability scanning
- [x] 2.3 Add grype vulnerability scanning
- [x] 2.4 Configure CRITICAL/HIGH vulnerability gates
- [x] 2.5 Create artifact archiving for scan reports

## Phase 3: DEB Packaging (Ubuntu 22.04)
- [x] 3.1 Create debian/control file
- [x] 3.2 Create debian/rules file
- [x] 3.3 Create debian/postinst script (user creation, permissions, systemd enable)
- [x] 3.4 Create debian/postrm script (user removal on purge)
- [x] 3.5 Create debian/preinst script

## Phase 4: RPM Packaging (Oracle Linux 8/9)
- [x] 4.1 Create seaweedfs.spec file
- [x] 4.2 Define %pre script (user creation)
- [x] 4.3 Define %post script (permissions, systemd enable)
- [x] 4.4 Define %preun/%postun scripts (user removal on purge)

## Phase 5: Systemd Templates
- [x] 5.1 Create weed-master@.service template
- [x] 5.2 Create weed-volume@.service template
- [x] 5.3 Create weed-filer@.service template
- [x] 5.4 Create weed-s3@.service template
- [x] 5.5 Apply STIG-compliant hardening settings

## Phase 6: Configuration Management
- [x] 6.1 Create /etc/default/weed/weed.conf template
- [x] 6.2 Create startup script to read config and enable instances
- [x] 6.3 Create example override configs (/etc/weed/master-1.toml, etc.)

## Phase 7: HAProxy Integration
- [x] 7.1 Create /etc/haproxy/conf.d/weed-s3.cfg
- [x] 7.2 Configure backend pool for S3 instances
- [x] 7.3 Add health checks and round-robin balancing

## Phase 8: Logging
- [x] 8.1 Create /etc/logrotate.d/weed configuration
- [x] 8.2 Ensure log directory ownership by weed user
- [x] 8.3 Configure daily rotation, 14 days retention

## Phase 9: Artifact Management
- [x] 9.1 Generate SHA256 checksums for all artifacts
- [x] 9.2 Create GPG detached signatures
- [x] 9.3 Archive deb packages, rpm packages, SBOMs, scan reports

## Phase 10: CI/CD Pipeline
- [x] 10.1 Create .github/workflows/build.yml
- [x] 10.2 Implement tag detection trigger
- [x] 10.3 Configure matrix build for all architectures
- [x] 10.4 Add stage: Build → SBOM → Scan → Package → Sign → Archive

## Phase 11: Additional Enhancements
- [ ] 11.1 Create SELinux policy module for Oracle Linux
- [ ] 11.2 Create AppArmor profile for Ubuntu
- [ ] 11.3 Create /opt/weed/healthcheck.sh script
- [ ] 11.4 Create pre-flight validation tool
- [ ] 11.5 Add port conflict detection
- [ ] 11.6 Configure graceful shutdown support
- [ ] 11.7 Add systemd watchdog integration
- [ ] 11.8 Set immutable binary permissions (chmod 0755 root:root)

## Phase 12: Documentation
- [ ] 12.1 Document directory structure
- [ ] 12.2 Document signing instructions
- [ ] 12.3 Create example configuration files
- [ ] 12.4 Document compliance features
