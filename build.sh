#!/bin/bash

COMMIT_SHA=$(git rev-parse HEAD)
VERSION=$(git describe --tags)
BUILD_TIME=$(date +'%Y-%m-%d %T')

LDFlags="\
    -s -w \
    -X 'config.Version=${VERSION}' \
    -X 'config.CommitSHA=${COMMIT_SHA}' \
    -X 'config.BuildTime=${BUILD_TIME}' \
    -X 'config.Repo=XiaoMengXinX/Music163bot-Go'\
"

CGO_ENABLED=0 go build -trimpath -ldflags "${LDFlags}"
