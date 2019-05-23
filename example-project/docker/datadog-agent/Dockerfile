FROM datadog/agent:latest

LABEL maintainer="lee@geeksinthewoods.com"

#COPY target/go_expvar.conf.yaml /etc/datadog-agent/conf.d/go_expvar.d/conf.yaml
COPY target/custom-init.sh /custom-init.sh

ARG service
ENV SERVICE_NAME $service

ARG env="dev"
ENV ENV $env

ENV DD_TAGS="source:docker service:${service} service_name:${service} cluster:NA env:${ENV}"

CMD ["/custom-init.sh"]
