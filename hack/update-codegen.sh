#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly SCRIPT_ROOT="$(cd "$(dirname "${BASH_SOURCE}")"/.. && pwd)"

# Keep outer module cache so we don't need to redownload them each time.
# The build cache already is persisted.
readonly GOMODCACHE="$(go env GOMODCACHE)"
readonly GO111MODULE="on"
readonly GOFLAGS="-mod=readonly"
readonly GOPATH="$(mktemp -d)"
readonly MIN_REQUIRED_GO_VER="$(go list -m -f '{{.GoVersion}}')"

function go_version_matches {
  go version | perl -ne "exit 1 unless m{go version go([0-9]+.[0-9]+)}; exit 1 if (\$1 < ${MIN_REQUIRED_GO_VER})"
  return $?
}

if ! go_version_matches; then
  echo "Go v${MIN_REQUIRED_GO_VER} or later is required to run code generation"
  exit 1
fi

export GOMODCACHE GO111MODULE GOFLAGS GOPATH


# Define overall configuration.
readonly REPO="ricochet/polaris"
readonly APIS_DIR="apis"
# Define the output package for the generated code.
readonly OUTPUT_PKG="${REPO}/pkg/client"
# Define clientset configuration.
readonly CLIENTSET_NAME="versioned"
readonly CLIENTSET_PKG_NAME="clientset"


# Define the versions of the API for which code generation is to be performed.
readonly VERSIONS=(v1alpha1)
INPUT_DIRS=""
for VERSION in "${VERSIONS[@]}"
do
  INPUT_DIRS+="${REPO}/${APIS_DIR}/${VERSION},"
done
readonly INPUT_DIRS="${INPUT_DIRS%,}" # drop trailing comma

# Allow for running in verification mode.
if [[ "${VERIFY_CODEGEN:-}" == "true" ]]; then
  echo "Running in verification mode"
  readonly VERIFY_FLAG="--verify-only"
fi

# Construct the common flags for code generation.
# Always use the boilerplate from the hack directory.
readonly COMMON_FLAGS="${VERIFY_FLAG:-} --go-header-file ${SCRIPT_ROOT}/hack/boilerplate.go.txt"


# # throw away
# new_report="$(mktemp -t "$(basename "$0").api_violations.XXXXXX")"

# echo "Generating openapi schema"
# go run k8s.io/code-generator/cmd/openapi-gen \
#   -O zz_generated.openapi \
#   --report-filename "${new_report}" \
#   --output-package "${REPO}/pkg/generated/openapi" \
#   --input-dirs "${INPUT_DIRS}" \
#   --input-dirs "k8s.io/apimachinery/pkg/apis/meta/v1" \
#   --input-dirs "k8s.io/apimachinery/pkg/runtime" \
#   --input-dirs "k8s.io/apimachinery/pkg/version" \
#   ${COMMON_FLAGS}

# echo "Generating apply configuration"
# go run k8s.io/code-generator/cmd/applyconfiguration-gen \
#   --input-dirs "${INPUT_DIRS}" \
#   --openapi-schema <(go run ${SCRIPT_ROOT}/cmd/modelschema) \
#   --output-package "${APIS_PKG}/apis/applyconfiguration" \
#   ${COMMON_FLAGS}


echo "Generating clientset at ${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}/"
go run k8s.io/code-generator/cmd/client-gen \
    --clientset-name "${CLIENTSET_NAME}" \
    --input-base '' \
    --input "${INPUT_DIRS}" \
    --output-base pkg/ \
    --output-package "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}/" \
    --trim-path-prefix "pkg/${REPO}/" \
    ${COMMON_FLAGS}


echo "Generating listers at ${OUTPUT_PKG}/listers"
go run k8s.io/code-generator/cmd/lister-gen \
  --input-dirs "${INPUT_DIRS}" \
  --output-package "${OUTPUT_PKG}/listers" \
  ${COMMON_FLAGS}

echo "Generating informers at ${OUTPUT_PKG}/informers"
go run k8s.io/code-generator/cmd/informer-gen \
  --input-dirs "${INPUT_DIRS}" \
  --versioned-clientset-package "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}/${CLIENTSET_NAME}" \
  --listers-package "${OUTPUT_PKG}/listers" \
  --output-package "${OUTPUT_PKG}/informers" \
  ${COMMON_FLAGS}

echo "Generating ${VERSION} register at ${REPO}/${APIS_DIR}/${VERSION}"
go run k8s.io/code-generator/cmd/register-gen \
  --input-dirs "${INPUT_DIRS}" \
  --output-package "${APIS_DIR}" \
  ${COMMON_FLAGS}


echo "Generating clientset at ${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}/"
go run k8s.io/code-generator/cmd/client-gen \
    --clientset-name "${CLIENTSET_NAME}" \
    --input-base '' \
    --input "${INPUT_DIRS}" \
    --output-base pkg/ \
    --output-package "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}/" \
    --trim-path-prefix "pkg/${REPO}/" \
    ${COMMON_FLAGS}