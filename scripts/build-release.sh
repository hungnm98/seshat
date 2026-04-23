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
rm -f "${dist_dir}/seshat_${version}_"* "${dist_dir}/SHA256SUMS"

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
  label="${os}"
  if [[ "${os}" == "darwin" ]]; then
    label="macos"
  fi
  name="seshat_${version}_${label}_${arch}"
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

if command -v lipo >/dev/null 2>&1; then
  universal_name="seshat_${version}_macos_universal"
  universal_dir="${build_dir}/${universal_name}"
  universal_archive="${dist_dir}/${universal_name}.tar.gz"

  mkdir -p "${universal_dir}"
  echo "build-release: building ${universal_name}"
  lipo -create \
    "${build_dir}/seshat_${version}_macos_amd64/seshat" \
    "${build_dir}/seshat_${version}_macos_arm64/seshat" \
    -output "${universal_dir}/seshat"
  cp "${repo_root}/VERSION" "${universal_dir}/VERSION"
  rm -f "${universal_archive}"
  (cd "${build_dir}" && tar -czf "${universal_archive}" "${universal_name}")
else
  echo "build-release: lipo not found; skipping darwin universal artifact" >&2
fi

(
  cd "${dist_dir}"
  shasum -a 256 "seshat_${version}_"* > SHA256SUMS
)

echo "build-release: artifacts"
ls -lh "${dist_dir}"/"seshat_${version}_"* "${dist_dir}/SHA256SUMS"
