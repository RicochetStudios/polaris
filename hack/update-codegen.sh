#!/usr/bin/env bash
#
# Helpful links:
# https://github.com/kubernetes-sigs/kubebuilder/issues/3795#issuecomment-1959266225
# https://github.com/kubernetes-sigs/gateway-api/blob/main/hack/update-codegen.sh
# https://github.com/kubernetes/community/blob/master/contributors/devel/sig-api-machinery/generating-clientset.md

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
readonly CODEGEN_VERSION="v0.30.0"

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
readonly REPO="github.com/RicochetStudios/polaris"
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


# throw away
new_report="$(mktemp -t "$(basename "$0").api_violations.XXXXXX")"

echo "Generating openapi schema"
# God knows what version to use here. I'm just going to use the latest.
go run k8s.io/kube-openapi/cmd/openapi-gen@latest \
  --output-file zz_generated.openapi.go \
  --report-filename "${new_report}" \
  --output-dir "pkg/generated/openapi" \
  --output-pkg "${REPO}/pkg/generated/openapi" \
  ${COMMON_FLAGS} \
  ${INPUT_DIRS} \
  k8s.io/apimachinery/pkg/apis/meta/v1 \
  k8s.io/apimachinery/pkg/runtime \
  k8s.io/apimachinery/pkg/version

# I think this doesn't work, due to a bug here:
# https://github.com/kubernetes/kubernetes/pull/124371
# echo "Generating apply configuration"
# go run k8s.io/code-generator/cmd/applyconfiguration-gen@$CODEGEN_VERSION \
#   --openapi-schema <(go run ${SCRIPT_ROOT}/cmd/modelschema) \
#   --output-dir "apis/applyconfiguration" \
#   --output-pkg "${REPO}/apis/applyconfiguration" \
#   ${COMMON_FLAGS} \
#   ${INPUT_DIRS}


echo "Generating clientset at ${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}/"
go run k8s.io/code-generator/cmd/client-gen@$CODEGEN_VERSION \
    --clientset-name "${CLIENTSET_NAME}" \
    --input-base '' \
    --input "${INPUT_DIRS}" \
    --output-dir "pkg/client/${CLIENTSET_PKG_NAME}" \
    --output-pkg "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}/" \
    ${COMMON_FLAGS}


echo "Generating listers at ${OUTPUT_PKG}/listers"
go run k8s.io/code-generator/cmd/lister-gen@$CODEGEN_VERSION \
  --output-dir "pkg/client/listers" \
  --output-pkg "${OUTPUT_PKG}/listers" \
  ${COMMON_FLAGS} \
  ${INPUT_DIRS}

echo "Generating informers at ${OUTPUT_PKG}/informers"
go run k8s.io/code-generator/cmd/informer-gen@$CODEGEN_VERSION \
  --versioned-clientset-package "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}/${CLIENTSET_NAME}" \
  --listers-package "${OUTPUT_PKG}/listers" \
  --output-dir "pkg/client/informers" \
  --output-pkg "${OUTPUT_PKG}/informers" \
  ${COMMON_FLAGS} \
  ${INPUT_DIRS}

echo "Generating ${VERSION} register at ${REPO}/${APIS_DIR}/${VERSION}"
go run k8s.io/code-generator/cmd/register-gen@$CODEGEN_VERSION \
  --output-file zz_generated.register.go \
  ${COMMON_FLAGS} \
  ${INPUT_DIRS}


echo "Generating clientset at ${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}/"
go run k8s.io/code-generator/cmd/client-gen@$CODEGEN_VERSION \
    --clientset-name "${CLIENTSET_NAME}" \
    --input-base '' \
    --input "${INPUT_DIRS}" \
    --output-dir "pkg/client/${CLIENTSET_PKG_NAME}" \
    --output-pkg "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}/" \
    ${COMMON_FLAGS}