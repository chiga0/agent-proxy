FROM ubuntu:22.04

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends squid curl && \
    rm -rf /var/lib/apt/lists/* && \
    mkdir -p /var/log/squid

COPY squid-tunnel.conf /etc/squid/squid-tunnel.conf
COPY squid-direct.conf /etc/squid/squid-direct.conf
COPY run-acl-tests.sh /usr/local/bin/run-acl-tests.sh
RUN chmod +x /usr/local/bin/run-acl-tests.sh
