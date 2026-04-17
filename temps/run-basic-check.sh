#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK_DIR="${ROOT_DIR}/temps/runtime-check"
BINARY_PATH="${WORK_DIR}/oci-sync"
TEST_REPO="${OCI_SYNC_TEST_REPO:-internal.183867412.xyz:5000/file/share}"
BASE_TAG="${OCI_SYNC_TEST_TAG_BASE:-runtime-check-$(date +%s)}"
ENCRYPTED_PASSPHRASE="${OCI_SYNC_TEST_PASSPHRASE:-runtime-check-secret}"
KEEP_WORKDIR="${OCI_SYNC_KEEP_WORKDIR:-0}"

cleanup_refs=()
run_succeeded=0

cleanup() {
  local remote_ref
  for remote_ref in "${cleanup_refs[@]}"; do
    [[ -n "${remote_ref}" ]] || continue
    echo "Cleaning up remote ref ${remote_ref}..."
    "${BINARY_PATH}" delete --remote "${remote_ref}" >/dev/null 2>&1 || true
  done

  if [[ "${KEEP_WORKDIR}" == "1" ]]; then
    echo "Keeping work directory ${WORK_DIR} because OCI_SYNC_KEEP_WORKDIR=1."
    return
  fi

  if [[ "${run_succeeded}" == "1" ]]; then
    rm -rf "${WORK_DIR}"
  else
    echo "Keeping work directory ${WORK_DIR} for debugging because the script did not finish successfully."
  fi
}

remote_ref_for_tag() {
  local tag="$1"
  printf '%s:%s' "${TEST_REPO}" "${tag}"
}

remove_cleanup_ref() {
  local target="$1"
  local remaining=()
  local remote_ref

  for remote_ref in "${cleanup_refs[@]}"; do
    if [[ "${remote_ref}" != "${target}" ]]; then
      remaining+=("${remote_ref}")
    fi
  done

  cleanup_refs=("${remaining[@]}")
}

trap cleanup EXIT

build_binary() {
  echo "Building oci-sync binary..."
  mkdir -p "${WORK_DIR}"
  (
    cd "${ROOT_DIR}"
    go build -o "${BINARY_PATH}" .
  )
}

setup_shortcut_config() {
  echo "Setting up shortcut config..."
  cat > "${WORK_DIR}/oci-sync.yaml" << EOF
shortcuts:
  x:
    repo: ${TEST_REPO}
EOF
}

prepare_test_data() {
  local source_dir="$1"
  local output_dir="$2"
  local marker="$3"

  echo "Preparing test data under ${source_dir}..."
  rm -rf "${source_dir}" "${output_dir}"
  mkdir -p "${source_dir}/sub" "${output_dir}"
  printf 'runtime-check-%s\n' "${marker}" > "${source_dir}/hello.txt"
  printf 'nested-content-%s\n' "${marker}" > "${source_dir}/sub/nested.txt"
}

push_artifact() {
  local family="$1"
  local source_dir="$2"
  local tag="$3"
  local passphrase="$4"
  local remote_ref

  remote_ref="$(remote_ref_for_tag "${tag}")"
  if [[ "${family}" == "standard" ]]; then
    if [[ -n "${passphrase}" ]]; then
      "${BINARY_PATH}" push --local "${source_dir}" --remote "${remote_ref}" --passphrase "${passphrase}"
    else
      "${BINARY_PATH}" push --local "${source_dir}" --remote "${remote_ref}"
    fi
    return
  fi

  if [[ -n "${passphrase}" ]]; then
    (cd "${WORK_DIR}" && "${BINARY_PATH}" x push --local "${source_dir}" --tag "${tag}" --passphrase "${passphrase}")
  else
    (cd "${WORK_DIR}" && "${BINARY_PATH}" x push --local "${source_dir}" --tag "${tag}")
  fi
}

list_artifacts() {
  local family="$1"

  if [[ "${family}" == "standard" ]]; then
    "${BINARY_PATH}" list --remote "${TEST_REPO}"
    return
  fi

  (cd "${WORK_DIR}" && "${BINARY_PATH}" x list)
}

pull_artifact() {
  local family="$1"
  local tag="$2"
  local output_dir="$3"
  local passphrase="$4"
  local remote_ref

  remote_ref="$(remote_ref_for_tag "${tag}")"
  if [[ "${family}" == "standard" ]]; then
    if [[ -n "${passphrase}" ]]; then
      "${BINARY_PATH}" pull --remote "${remote_ref}" --local "${output_dir}" --passphrase "${passphrase}"
    else
      "${BINARY_PATH}" pull --remote "${remote_ref}" --local "${output_dir}"
    fi
    return
  fi

  if [[ -n "${passphrase}" ]]; then
    (cd "${WORK_DIR}" && "${BINARY_PATH}" x pull --tag "${tag}" --local "${output_dir}" --passphrase "${passphrase}")
  else
    (cd "${WORK_DIR}" && "${BINARY_PATH}" x pull --tag "${tag}" --local "${output_dir}")
  fi
}

delete_artifact() {
  local family="$1"
  local tag="$2"
  local remote_ref

  remote_ref="$(remote_ref_for_tag "${tag}")"
  if [[ "${family}" == "standard" ]]; then
    "${BINARY_PATH}" delete --remote "${remote_ref}"
    return
  fi

  (cd "${WORK_DIR}" && "${BINARY_PATH}" x delete --tag "${tag}")
}

run_case() {
  local family="$1"
  local case_name="$2"
  local tag="$3"
  local passphrase="${4:-}"
  local source_dir="${WORK_DIR}/${family}-${case_name}-src"
  local output_dir="${WORK_DIR}/${family}-${case_name}-out"
  local remote_ref
  local list_output

  remote_ref="$(remote_ref_for_tag "${tag}")"

  prepare_test_data "${source_dir}" "${output_dir}" "${tag}"

  echo "Pushing ${family}/${case_name} artifact to ${remote_ref}..."
  push_artifact "${family}" "${source_dir}" "${tag}" "${passphrase}"
  cleanup_refs+=("${remote_ref}")

  echo "Listing repository tags via ${family} command..."
  list_output="$(list_artifacts "${family}")"
  printf '%s\n' "${list_output}"
  if [[ "${list_output}" != *"${tag}"* ]]; then
    echo "Expected tag ${tag} was not found in list output." >&2
    return 1
  fi

  echo "Pulling ${family}/${case_name} artifact back to ${output_dir}..."
  pull_artifact "${family}" "${tag}" "${output_dir}" "${passphrase}"

  echo "Validating ${family}/${case_name} content..."
  test -f "${output_dir}/$(basename "${source_dir}")/hello.txt"
  test -f "${output_dir}/$(basename "${source_dir}")/sub/nested.txt"
  cmp "${source_dir}/hello.txt" "${output_dir}/$(basename "${source_dir}")/hello.txt"
  cmp "${source_dir}/sub/nested.txt" "${output_dir}/$(basename "${source_dir}")/sub/nested.txt"

  echo "Deleting ${family}/${case_name} artifact..."
  delete_artifact "${family}" "${tag}"
  remove_cleanup_ref "${remote_ref}"

  echo "${family}/${case_name} runtime check passed."
}

build_binary
setup_shortcut_config
run_case "standard" "plain" "${BASE_TAG}-standard-plain" ""
run_case "standard" "encrypted" "${BASE_TAG}-standard-encrypted" "${ENCRYPTED_PASSPHRASE}"
run_case "x" "plain" "${BASE_TAG}-x-plain" ""
run_case "x" "encrypted" "${BASE_TAG}-x-encrypted" "${ENCRYPTED_PASSPHRASE}"
run_succeeded=1

echo "All runtime checks passed."
echo "Repo: ${TEST_REPO}"
echo "Tags: ${BASE_TAG}-standard-plain, ${BASE_TAG}-standard-encrypted, ${BASE_TAG}-x-plain, ${BASE_TAG}-x-encrypted"