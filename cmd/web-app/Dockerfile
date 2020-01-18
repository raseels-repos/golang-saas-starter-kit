# To manully execute build locally use:
#   docker build -f cmd/web-app/Dockerfile  --build-arg name=web-app .
# This uses a multi-stage docker file. You can use target to build a specific stage.
#   docker build -f cmd/web-app/Dockerfile  --build-arg name=web-app --target dev .

FROM golang:1.13.5-alpine3.11 AS build_base_golang

LABEL maintainer="lee@geeksinthewoods.com"

RUN apk --update --no-cache add \
            git

# Change dir to project base.
WORKDIR $GOPATH/src/gitlab.com/geeks-accelerator/oss/saas-starter-kit

# Build and (re)start go apps after saving/creating/deleting source files.
RUN GO111MODULE=off go get github.com/pilu/fresh

# Enable go modules.
ARG GOPROXY=https://goproxy.io
ENV GOPROXY=$GOPROXY
ENV GO111MODULE="on"
COPY go.mod .
COPY go.sum .
RUN go mod download

FROM build_base_golang AS dev

ARG name
ARG code_path=./cmd/${name}
ARG commit_ref=-

ARG name
ENV SERVICE_NAME $name

ARG env="dev"
ENV ENV $env

ARG gogc="20"
ENV GOGC $gogc

# Copy shared packages.
COPY internal ./internal

# Copy cmd specific packages.
COPY ${code_path} ${code_path}
COPY ${code_path}/templates /build/templates
COPY ${code_path}/static /build/static

# Copy the global templates.
ADD resources/templates/shared /build/templates/shared
ADD configs/fresh-auto-reload.conf /build/runner.conf

# These need to be relative paths for Hot reloads to work with docker-compose.
ENV TEMPLATE_DIR=./templates
ENV SHARED_TEMPLATE_DIR=../../resources/templates/shared
ENV STATIC_DIR=./static

WORKDIR ${code_path}

ENTRYPOINT ["fresh", "-c", "/build/runner.conf"]

FROM dev AS builder

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.build=${commit_ref}" -a -installsuffix nocgo -o /gosrv .

FROM alpine:3.11

RUN apk --update --no-cache add \
            tzdata ca-certificates curl openssl

COPY --from=builder /build /

ENV TEMPLATE_DIR=/templates
ENV SHARED_TEMPLATE_DIR=/templates/shared
ENV STATIC_DIR=/static

ARG name
ENV SERVICE_NAME $name

ARG env="dev"
ENV ENV $env

ARG gogc="20"
ENV GOGC $gogc

ENTRYPOINT ["/gosrv"]
