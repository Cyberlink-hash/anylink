#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/anylink-web.XXXXXX")
container_id=""
cleanup() {
  if [[ -n "$container_id" ]]; then
    docker container rm "$container_id" >/dev/null || true
  fi
  rm -rf "$work_dir"
}
trap cleanup EXIT

docker build --pull --progress=plain --iidfile "$work_dir/image.id" \
  -f docker/Dockerfile.web .
container_id=$(docker container create --pull=never --network=none "$(cat "$work_dir/image.id")")
docker cp "$container_id:/ui" "$work_dir/ui"
test -s "$work_dir/ui/index.html"

rm -rf web/ui server/ui
cp -R "$work_dir/ui" web/ui
cp -R "$work_dir/ui" server/ui
echo "前端编译完成: web/ui、server/ui"
