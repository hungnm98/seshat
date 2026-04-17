#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

output="${1:-${SESHAT_BIN:-${HOME}/.local/bin/seshat}}"
output_dir="$(dirname "${output}")"

mkdir -p "${output_dir}"

echo "build-local: building seshat CLI"
echo "build-local: repo=${repo_root}"
echo "build-local: output=${output}"

(
  cd "${repo_root}/cli"
  go build -o "${output}" ./cmd/seshat
)

echo "build-local: built ${output}"
"${output}" 2>&1 | sed -n '1,12p'
