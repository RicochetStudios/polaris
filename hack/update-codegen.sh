#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# readonly SCRIPT_ROOT="$(cd "$(dirname "${BASH_SOURCE}")"/.. && pwd)"

# # Keep outer module cache so we don't need to redownload them each time.
# # The build cache already is persisted.
# readonly GOMODCACHE="$(go env GOMODCACHE)"
# readonly GO111MODULE="on"
# readonly GOFLAGS="-mod=readonly"
# readonly GOPATH="$(mktemp -d)"
# readonly MIN_REQUIRED_GO_VER="$(go list -m -f '{{.GoVersion}}')"

# function go_version_matches {
#   go version | perl -ne "exit 1 unless m{go version go([0-9]+.[0-9]+)}; exit 1 if (\$1 < ${MIN_REQUIRED_GO_VER})"
#   return $?
# }

# if ! go_version_matches; then
#   echo "Go v${MIN_REQUIRED_GO_VER} or later is required to run code generation"
#   exit 1
# fi

# export GOMODCACHE GO111MODULE GOFLAGS GOPATH

# # Even when modules are enabled, the code-generator tools always write to
# # a traditional GOPATH directory, so fake on up to point to the current
# # workspace.
# mkdir -p "$GOPATH/src/sigs.k8s.io"
# ln -s "${SCRIPT_ROOT}" "$GOPATH/src/sigs.k8s.io/gateway-api"

# readonly OUTPUT_PKG=sigs.k8s.io/gateway-api/pkg/client
# readonly APIS_PKG=sigs.k8s.io/gateway-api
# readonly CLIENTSET_NAME=versioned
# readonly CLIENTSET_PKG_NAME=clientset
# readonly VERSIONS=(v1alpha2 v1beta1 v1)

# GATEWAY_INPUT_DIRS=""
# for VERSION in "${VERSIONS[@]}"
# do
#   GATEWAY_INPUT_DIRS+="${APIS_PKG}/apis/${VERSION},"
# done
# readonly GATEWAY_INPUT_DIRS="${GATEWAY_INPUT_DIRS%,}" # drop trailing comma


# if [[ "${VERIFY_CODEGEN:-}" == "true" ]]; then
#   echo "Running in verification mode"
#   readonly VERIFY_FLAG="--verify-only"
# fi

# readonly COMMON_FLAGS="${VERIFY_FLAG:-} --go-header-file ${SCRIPT_ROOT}/hack/boilerplate/boilerplate.generatego.txt"

# echo "Generating CRDs"
# go run ./pkg/generator

# # throw away
# new_report="$(mktemp -t "$(basename "$0").api_violations.XXXXXX")"

# echo "Generating openapi schema"
# go run k8s.io/code-generator/cmd/openapi-gen \
#   -O zz_generated.openapi \
#   --report-filename "${new_report}" \
#   --output-package "sigs.k8s.io/gateway-api/pkg/generated/openapi" \
#   --input-dirs "${GATEWAY_INPUT_DIRS}" \
#   --input-dirs "k8s.io/apimachinery/pkg/apis/meta/v1" \
#   --input-dirs "k8s.io/apimachinery/pkg/runtime" \
#   --input-dirs "k8s.io/apimachinery/pkg/version" \
#   ${COMMON_FLAGS}

# echo "Generating apply configuration"
# go run k8s.io/code-generator/cmd/applyconfiguration-gen \
#   --input-dirs "${GATEWAY_INPUT_DIRS}" \
#   --openapi-schema <(go run ${SCRIPT_ROOT}/cmd/modelschema) \
#   --output-package "${APIS_PKG}/apis/applyconfiguration" \
#   ${COMMON_FLAGS}

# echo "Generating clientset at ${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}"
# go run k8s.io/code-generator/cmd/client-gen \
#   --clientset-name "${CLIENTSET_NAME}" \
#   --input-base "${APIS_PKG}" \
#   --input "${GATEWAY_INPUT_DIRS//${APIS_PKG}/}" \
#   --output-package "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}" \
#   --apply-configuration-package "${APIS_PKG}/apis/applyconfiguration" \
#   ${COMMON_FLAGS}

# echo "Generating listers at ${OUTPUT_PKG}/listers"
# go run k8s.io/code-generator/cmd/lister-gen \
#   --input-dirs "${GATEWAY_INPUT_DIRS}" \
#   --output-package "${OUTPUT_PKG}/listers" \
#   ${COMMON_FLAGS}

# echo "Generating informers at ${OUTPUT_PKG}/informers"
# go run k8s.io/code-generator/cmd/informer-gen \
#   --input-dirs "${GATEWAY_INPUT_DIRS}" \
#   --versioned-clientset-package "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}/${CLIENTSET_NAME}" \
#   --listers-package "${OUTPUT_PKG}/listers" \
#   --output-package "${OUTPUT_PKG}/informers" \
#   ${COMMON_FLAGS}

# echo "Generating ${VERSION} register at ${APIS_PKG}/apis/${VERSION}"
# go run k8s.io/code-generator/cmd/register-gen \
#   --input-dirs "${GATEWAY_INPUT_DIRS}" \
#   --output-package "${APIS_PKG}/apis" \
#   ${COMMON_FLAGS}


# echo "Generating clientset at ${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}"
# go run k8s.io/code-generator/cmd/client-gen \
#   --clientset-name "${CLIENTSET_NAME}" \
#   --input-base "${APIS_PKG}" \
#   --input "${GATEWAY_INPUT_DIRS//${APIS_PKG}/}" \
#   --output-package "${OUTPUT_PKG}/${CLIENTSET_PKG_NAME}" \
#   --apply-configuration-package "${APIS_PKG}/apis/applyconfiguration" \
#   ${COMMON_FLAGS}

readonly REPO="github.com/RicochetStudios/polaris"
readonly APIS_DIR="apis"

go run k8s.io/code-generator/cmd/client-gen \
    --go-header-file ./hack/boilerplate.go.txt \
    --clientset-name clientset \
    --input-base '' \
    --input "${REPO}/${APIS_DIR}/v1" \
    --output-base pkg/ \
    --output-package "${REPO}/pkg/" \
    --trim-path-prefix "pkg/${REPO}/"