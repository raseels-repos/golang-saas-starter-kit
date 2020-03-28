#!/bin/bash

# Parse project path from the git config.
#	url = git@gitlab.com:geeks-accelerator/oss/saas-starter-kit.git
repoURL=$(grep "url =" .git/config | head -n 1 | awk -F '= ' '{print $2}')

echo -e "Respository URL: \n${repoURL}"

read -p "Is this correct (y/n)?" choice
case "$choice" in
  y|Y ) echo "yes";;
  n|N ) echo "no"; exit;;
  * ) echo "invalid";;
esac

gitHost=$(echo $repoURL | awk -F '@' '{print $2}' | awk -F ':' '{print $1}')

projectPath=$(echo $repoURL | awk -F ':' '{print $2}' | awk -F '.' '{print $1}')
projectName=$(basename ${projectPath})

echo "gitHost: ${gitHost}"
echo "projectPath: ${projectPath}"
echo "projectName: ${projectName}"

docker login registry.gitlab.com
cd build/docker/golang/1.13/docker && docker build -t golang1.13-docker -t registry.${gitHost}/${projectPath}:golang1.13-docker .
docker push registry.${gitHost}/${projectPath}:golang1.13-docker

flist=`grep -r "gitlab.com:geeks-accelerator/oss/saas-starter-kit" * | grep -v setup.sh | awk -F ':' '{print $1}' | sort | uniq`
for f in $flist; do echo $f; sed -i "" -e "s|gitlab.com:geeks-accelerator/oss/saas-starter-kit|${gitHost}:geeks-accelerator/oss/saas-starter-kit|g" $f; done

flist=`grep -r "gitlab.com:geeks-accelerator/oss/saas-starter-kit" * | grep -v setup.sh | awk -F ':' '{print $1}' | sort | uniq`
for f in $flist; do echo $f; sed -i "" -e "s|geeks-accelerator/oss/saas-starter-kit|${projectPath}|g" $f; done

flist=`grep -r "saas-starter-kit" * | grep -v setup.sh | awk -F ':' '{print $1}' | sort | uniq`
for f in $flist; do echo $f; sed -i "" -e "s|saas-starter-kit|${projectName}|g" $f; done

flist=`grep -r "saas-starter-kit" * | grep -v setup.sh | awk -F ':' '{print $1}' | sort | uniq`
for f in $flist; do echo $f; sed -i "" -e "s|saas-starter-kit|${projectName}|g" $f; done
