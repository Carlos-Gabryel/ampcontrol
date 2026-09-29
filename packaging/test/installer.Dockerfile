# Ubuntu com systemd para ensaiar scripts/install.sh no CI (scripts/test-installer.sh).
FROM ubuntu:24.04

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        systemd systemd-sysv sudo python3 util-linux ca-certificates \
    && rm -rf /var/lib/apt/lists/*

STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
