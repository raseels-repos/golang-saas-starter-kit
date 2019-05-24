#!/usr/bin/env bash

configFile="/etc/datadog-agent/conf.d/go_expvar.d/conf.yaml"

echo -e "init_config:\n\ninstances:\n" > $configFile

if [[ "${DD_EXPVAR}" != "" ]]; then

     while IFS='|' read -ra HOSTS; do
        for h in "${HOSTS[@]}"; do
            if [[ "${h}" == "" ]]; then
                continue
            fi

            url=""
            for p in $h; do
                k=`echo $p | awk -F '=' '{print $1}'`
                v=`echo $p | awk -F '=' '{print $2}'`
                if [[ "${k}" == "url" ]]; then
                    url=$v
                    break
                fi
            done

            if [[ "${url}" == "" ]]; then
                echo "No url param found in '${h}'"
                continue
            fi

            echo -e "  - expvar_url: ${url}" >> $configFile
            if [[ "${DD_TAGS}" != "" ]]; then
                echo "    tags:" >> $configFile
                for t in ${DD_TAGS}; do
                    echo "      - ${t}" >> $configFile
                done
            fi

            for p in $h; do
                k=`echo $p | awk -F '=' '{print $1}'`
                v=`echo $p | awk -F '=' '{print $2}'`
                if [[ "${k}" == "url" ]]; then
                    continue
                fi
                echo "      - ${k}:${v}" >> $configFile
            done
        done
     done <<< "$DD_EXPVAR"
else :
    echo -e "  - expvar_url: http://localhost:80/debug/vars" >> $configFile
    if [[ "${DD_TAGS}" != "" ]]; then
        echo "    tags:" >> $configFile
        for t in ${DD_TAGS}; do
            echo "      - ${t}" >> $configFile
        done
    fi
fi

cat $configFile

/init
