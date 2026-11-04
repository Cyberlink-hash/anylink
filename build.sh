#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"
ver=$(cat version)
[[ "$ver" =~ ^[a-zA-Z0-9][a-zA-Z0-9._-]*$ ]] || { echo "无效的版本号" >&2; exit 1; }
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/anylink-build.XXXXXX")
container_id=""
cleanup() {
  if [[ -n "$container_id" ]]; then
    docker container rm "$container_id" >/dev/null || true
  fi
  rm -rf "$work_dir"
}
trap cleanup EXIT

ANYLINK_IID_FILE="$work_dir/image.id" bash build_docker.sh
container_id=$(docker container create --pull=never --network=none "$(cat "$work_dir/image.id")")
docker cp "$container_id:/app" "$work_dir/anylink-deploy"
test -s "$work_dir/anylink-deploy/anylink"
tar -czf "$work_dir/anylink-deploy-$ver.tar.gz" -C "$work_dir" anylink-deploy

rm -rf anylink-deploy
mv "$work_dir/anylink-deploy" anylink-deploy
mv "$work_dir/anylink-deploy-$ver.tar.gz" .
echo "anylink 编译完成，目录: anylink-deploy（请单独配置 server.toml 和 TLS 证书）"
ls -lh anylink-deploy
