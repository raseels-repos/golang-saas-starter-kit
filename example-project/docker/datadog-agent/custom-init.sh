#!/usr/bin/env bash

configFile="/etc/datadog-agent/conf.d/go_expvar.d/conf.yaml"

echo -e "init_config:\n\ninstances:\n  - expvar_url: http://localhost:80/debug/vars" > $configFile

if [[ "${DD_TAGS}" != "" ]]; then
    echo "    tags:" >> $configFile
    for t in ${DD_TAGS}; do
        echo "      - \"${t}\"" >> $configFile
    done
fi

cat $configFile

/init
