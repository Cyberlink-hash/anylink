#!/bin/sh
set -eu

apk add --no-cache ca-certificates bash iproute2 tzdata iptables iptables-legacy inetutils-telnet
chmod +x /app/docker_entrypoint.sh
mkdir -p /app/log /app/conf/files
cp -r /app/conf /home/conf-bak
