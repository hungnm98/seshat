#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C
export LANG=C

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

version="${VERSION:-$(tr -d '[:space:]' < "${repo_root}/VERSION")}"
dist_dir="${DIST_DIR:-${repo_root}/dist}"
build_dir="${dist_dir}/build"

if [[ -z "${version}" ]]; then
  echo "build-release: VERSION is empty" >&2
  exit 1
fi

rm -rf "${build_dir}"
mkdir -p "${build_dir}"

echo "build-release: version=${version}"
echo "build-release: repo=${repo_root}"
echo "build-release: dist=${dist_dir}"

targets=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "windows/amd64"
)

for target in "${targets[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  name="seshat_${version}_${os}_${arch}"
  out_dir="${build_dir}/${name}"
  bin="seshat"
  archive="${dist_dir}/${name}.tar.gz"

  if [[ "${os}" == "windows" ]]; then
    bin="seshat.exe"
    archive="${dist_dir}/${name}.zip"
  fi

  mkdir -p "${out_dir}"
  echo "build-release: building ${name}"
  (
    cd "${repo_root}/cli"
    CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" go build -trimpath -ldflags="-s -w" -o "${out_dir}/${bin}" ./cmd/seshat
  )
  cp "${repo_root}/VERSION" "${out_dir}/VERSION"

  rm -f "${archive}"
  if [[ "${os}" == "windows" ]]; then
    (cd "${build_dir}" && zip -q -r "${archive}" "${name}")
  else
    (cd "${build_dir}" && tar -czf "${archive}" "${name}")
  fi
done

(
  cd "${dist_dir}"
  shasum -a 256 "seshat_${version}_"* > SHA256SUMS
)

echo "build-release: artifacts"
ls -lh "${dist_dir}"/"seshat_${version}_"* "${dist_dir}/SHA256SUMS"
