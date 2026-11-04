#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"
action=${1:-build}
case "$action" in
  build|cntest) ;;
  *) echo "用法: $0 [build|cntest]" >&2; exit 1 ;;
esac
ver=$(cat version)
[[ "$ver" =~ ^[a-zA-Z0-9][a-zA-Z0-9._-]*$ ]] || { echo "无效的版本号" >&2; exit 1; }
test -s web/ui/index.html || { echo "请先运行 bash build_web.sh" >&2; exit 1; }
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/anylink-image.XXXXXX")
trap 'rm -rf "$work_dir"' EXIT
commit_id=$(git rev-parse HEAD)

docker build --pull --progress=plain --iidfile "$work_dir/image.id" \
  --build-arg "appVer=$ver" --build-arg "commitId=$commit_id" \
  -t "local/anylink:$ver" -f docker/Dockerfile .
image_id=$(cat "$work_dir/image.id")
if [[ -n "${ANYLINK_IID_FILE:-}" ]]; then
  cp "$work_dir/image.id" "$ANYLINK_IID_FILE"
fi

if [[ "$action" == "cntest" ]]; then
  target="registry.cn-hangzhou.aliyuncs.com/bjdgyc/anylink:test-$ver"
  docker tag "$image_id" "$target"
  docker push "$target"
fi
