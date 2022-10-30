#!/bin/bash

COMMIT_SHA=$(git rev-parse HEAD)
VERSION=$(git describe --tags)
BUILD_TIME=$(date +'%Y-%m-%d %T')

LDFlags="\
    -s -w \
    -X 'main._VersionName=${VERSION}' \
    -X 'main.commitSHA=${COMMIT_SHA}' \
    -X 'main.buildTime=${BUILD_TIME}' \
    -X 'main.repoPath=XiaoMengXinX/Music163bot-Go'\
    -X 'main.rawRepoPath=XiaoMengXinX/Music163bot-Go/v2'\
"

CGO_ENABLED=0 go build -trimpath -ldflags "${LDFlags}"
