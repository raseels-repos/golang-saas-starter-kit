#!/usr/bin/env bash

doPush=0
if [[ "${CI_REGISTRY_IMAGE}" != "" ]]; then
    docker login -u ${CI_REGISTRY_USER} -p ${CI_REGISTRY_PASSWORD} ${CI_REGISTRY}

    releaseImg="${CI_REGISTRY_IMAGE}:devops-${CI_COMMIT_REF_NAME}"
    doPush=1
else :
    releaseImg="devops"
fi

echo "release image: ${releaseImg}"

docker pull ${releaseImg} || true

docker build -f tools/devops/Dockerfile --cache-from ${releaseImg} -t ${releaseImg} .

if [[ $doPush == 1 ]]; then
    docker push ${releaseImg}
fi

docker run --rm --entrypoint=cat ${releaseImg} /devops > devops
chmod +x devops
