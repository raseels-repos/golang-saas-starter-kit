FROM golang:1.12.6-alpine3.9 AS build_base_golang

LABEL maintainer="lee@geeksinthewoods.com"

RUN apk --update --no-cache add \
            git

# Change dir to project base.
WORKDIR $GOPATH/src/gitlab.com/geeks-accelerator/oss/saas-starter-kit

# Enable go modules.
ENV GO111MODULE="on"
COPY go.mod .
COPY go.sum .
RUN go mod download
RUN go get github.com/pilu/fresh

FROM build_base_golang AS dev

ARG service
ARG commit_ref=-

# Copy shared packages.
COPY internal ./internal

# Copy cmd specific packages.
COPY cmd/${service} ./cmd/${service}
COPY cmd/${service}/templates /templates
COPY cmd/${service}/static /static

# Copy the global templates.
ADD resources/templates/shared /templates/shared
ADD fresh-auto-reload.conf /runner.conf

ENV TEMPLATE_DIR=/templates

WORKDIR ./cmd/${service}

ENTRYPOINT ["fresh", "-c", "/runner.conf"]

FROM dev AS builder

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.build=${commit_ref}" -a -installsuffix nocgo -o /gosrv .

FROM alpine:3.9

RUN apk --update --no-cache add \
            tzdata ca-certificates curl openssl

COPY --from=builder /gosrv /
COPY --from=builder /static /static
COPY --from=builder /templates /templates

ENV TEMPLATE_DIR=/templates
ENV SHARED_TEMPLATE_DIR=/templates/shared
ENV STATIC_DIR=/static

ARG service
ENV SERVICE_NAME $service

ARG env="dev"
ENV ENV $env

ARG gogc="20"
ENV GOGC $gogc

ENTRYPOINT ["/gosrv"]
