FROM golang:1.13.5-alpine3.11 AS build_base_golang

LABEL maintainer="lee@geeksinthewoods.com"

RUN apk --update --no-cache add \
            git build-base gcc

# Hack to get swag init to work correctly.
RUN GO111MODULE=off go get gopkg.in/go-playground/validator.v9 && \
    GO111MODULE=off go get github.com/go-playground/universal-translator && \
    GO111MODULE=off go get github.com/leodido/go-urn && \
    GO111MODULE=off go get github.com/lib/pq/oid && \
    GO111MODULE=off go get github.com/lib/pq/scram && \
    GO111MODULE=off go get github.com/tinylib/msgp/msgp && \
    GO111MODULE=off go get gopkg.in/DataDog/dd-trace-go.v1/ddtrace && \
    GO111MODULE=off go get github.com/xwb1989/sqlparser && \
    GO111MODULE=off go get golang.org/x/xerrors && \
    GO111MODULE=off go get github.com/pkg/errors && \
    GO111MODULE=off go get golang.org/x/crypto/nacl/secretbox

# Install swag with go modules enabled.
RUN GO111MODULE=on go get -u github.com/geeks-accelerator/swag/cmd/swag

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
ARG swagInit

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
COPY ${code_path}/templates /templates
#COPY ${code_path}/static /static

# Copy the global templates.
ADD resources/templates/shared /templates/shared
ADD configs/fresh-auto-reload.conf /runner.conf

ENV TEMPLATE_DIR=/templates
ENV SHARED_TEMPLATE_DIR=/templates/shared
#ENV STATIC_DIR=/static

WORKDIR ${code_path}

ENTRYPOINT ["fresh", "-c", "/runner.conf"]

FROM dev AS builder

# Update the API documentation.
# Disabled for the moment as it takes forever to run, rely on manual execution.
RUN if [ "$swagInit" != "" ]; then swag init ; fi

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.build=${commit_ref}" -a -installsuffix nocgo -o /gosrv .

FROM alpine:3.11

RUN apk --update --no-cache add \
            tzdata ca-certificates curl openssl

COPY --from=builder /gosrv /
#COPY --from=builder /static /static
COPY --from=builder /templates /templates

ENV TEMPLATE_DIR=/templates
ENV SHARED_TEMPLATE_DIR=/templates/shared
#ENV STATIC_DIR=/static

ARG name
ENV SERVICE_NAME $name

ARG env="dev"
ENV ENV $env

ARG gogc="20"
ENV GOGC $gogc

ENTRYPOINT ["/gosrv"]
