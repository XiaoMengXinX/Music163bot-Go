#!/bin/bash

COMMIT_SHA_S=$(git rev-parse --short HEAD)
COMMIT_SHA=$(git rev-parse HEAD)
VERSION=$(git describe --tags)
BUILD_TIME=$(date +'%Y-%m-%d %T')
BUILD_OS="$(lsb_release -i -s) $(lsb_release -r -s)"
BUILD_ARCH=$(uname -m)

LDFlags="\
    -s -w
    -X 'main.VERSION=${VERSION}' \
    -X 'main.COMMIT_SHA=${COMMIT_SHA}' \
    -X 'main.COMMIT_SHA_S=${COMMIT_SHA_S}' \
    -X 'main.BUILD_TIME=${BUILD_TIME}' \
    -X 'main.VERSION=${VERSION}' \
    -X 'main.BUILD_OS=${BUILD_OS}' \
"

go build -trimpath -ldflags "${LDFlags}"
