FROM golang:1.12.6-alpine3.9 AS build_base

LABEL maintainer="lee@geeksinthewoods.com"

RUN apk --update --no-cache add \
            git

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
COPY cmd/web-app ./cmd/web-app
COPY cmd/web-app/templates /templates
COPY cmd/web-app/static /static

WORKDIR ./cmd/web-app

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix nocgo -o /gosrv .

FROM alpine:3.9

RUN apk --update --no-cache add \
            tzdata ca-certificates curl openssl

COPY --from=builder /gosrv /
COPY --from=builder /static /static
COPY --from=builder /templates /templates

ARG service
ENV SERVICE_NAME $service

ARG env="dev"
ENV ENV $env

ARG gogc="20"
ENV GOGC $gogc

ENTRYPOINT ["/gosrv"]
