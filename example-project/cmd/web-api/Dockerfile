FROM golang:1.12.6-alpine3.9 AS build_base

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
    GO111MODULE=off go get gopkg.in/DataDog/dd-trace-go.v1/ddtrace

# Install swag with go modules enabled.
RUN GO111MODULE=on go get -u github.com/swaggo/swag/cmd/swag

# Change dir to project base.
WORKDIR $GOPATH/src/gitlab.com/geeks-accelerator/oss/saas-starter-kit/example-project

# Enable go modules.
ENV GO111MODULE="on"
COPY go.mod .
COPY go.sum .
RUN go mod download

FROM build_base AS builder

# Copy shared packages.
COPY internal ./internal

# Copy cmd specific packages.
COPY cmd/web-api ./cmd/web-api
COPY cmd/web-api/templates /templates
#COPY cmd/web-api/static /static

WORKDIR ./cmd/web-api

# Update the API documentation.
RUN swag init

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix nocgo -o /gosrv .

FROM alpine:3.9

RUN apk --update --no-cache add \
            tzdata ca-certificates curl openssl

COPY --from=builder /gosrv /
#COPY --from=builder /static /static
COPY --from=builder /templates /templates

ARG gogc="20"
ENV GOGC $gogc

ENTRYPOINT ["/gosrv"]
