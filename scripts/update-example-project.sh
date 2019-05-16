#!/bin/bash

GO111MODULE=off go get -u github.com/ardanlabs/service

scriptDir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" > /dev/null && pwd )"
exampleDir=${scriptDir}/../example-project/


if [[ -d ${exampleDir} ]]; then 
    echo "Deleted ${exampleDir}"
    rm -rf ${exampleDir} 
fi 

mkdir -p ${exampleDir}

cp -r $GOPATH/src/github.com/ardanlabs/service/* ${exampleDir}

flist=`grep -r "github.com/ardanlabs/service" example-project/ | awk -F ':' '{print $1}' | sort | uniq`
for f in $flist; do 
    echo $f;

    sed -i "" 's#github.com/ardanlabs/service#geeks-accelerator/oss/saas-starter-kit/example-project#g' $f;
done 

#rm -rf ${exampleDir}/vendor
rm -rf ${exampleDir}/models.xml
