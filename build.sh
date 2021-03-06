#!/bin/bash

COMMIT_SHA=$(git rev-parse HEAD)
VERSION=$(git describe --tags)
BUILD_TIME=$(date +'%Y-%m-%d %T')

LDFlags="\
    -s -w \
    -extldflags '-static -fpic' \
    -X 'main._VersionName=${VERSION}' \
    -X 'main._VersionCodeStr=${1}' \
    -X 'main.commitSHA=${COMMIT_SHA}' \
    -X 'main.buildTime=${BUILD_TIME}' \
    -X 'main.repoPath=XiaoMengXinX/Music163bot-Go'\
    -X 'main.rawRepoPath=XiaoMengXinX/Music163bot-Go/v2'\
"

go build -trimpath -ldflags "${LDFlags}"
