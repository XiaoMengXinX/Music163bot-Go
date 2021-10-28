#!/bin/bash

COMMIT_SHA=$(git rev-parse HEAD)
VERSION=$(git describe --tags)
BUILD_TIME=$(date +'%Y-%m-%d %T')

if which systeminfo >/dev/null; then
  BUILD_OS="$(systeminfo | grep "OS Name:" | sed -e "s/OS Name://" -e "s/  //g" -e "s/ //")"
elif which lsb_release >/dev/null; then
  BUILD_OS="$(lsb_release -i -s) $(lsb_release -r -s)"
else
  BUILD_OS="null"
fi

LDFlags="\
    -s -w
    -X 'main._VersionName=${VERSION}' \
    -X 'main._VersionCodeStr=${1}' \
    -X 'main.commitSHA=${COMMIT_SHA}' \
    -X 'main.buildTime=${BUILD_TIME}' \
    -X 'main.buildOS=${BUILD_OS}' \
    -X 'main.repoPath=XiaoMengXinX/Music163bot-Go'\
    -X 'main.rawRepoPath=XiaoMengXinX/Music163bot-Go/v2'\
"

go build -trimpath -ldflags "${LDFlags}"
