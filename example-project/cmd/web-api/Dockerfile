FROM golang:alpine3.9 AS build_base

LABEL maintainer="lee@geeksinthewoods.com"

RUN apk --update --no-cache add \
            git

# go to base project
WORKDIR $GOPATH/src/gitlab.com/geeks-accelerator/oss/saas-starter-kit/example-project

# enable go modules
ENV GO111MODULE="on"
COPY go.mod .
COPY go.sum .
RUN go mod download

FROM build_base AS builder

# copy shared packages
COPY internal ./internal

# copy cmd specific package
COPY cmd/web-api ./cmd/web-api
COPY cmd/web-api/templates /templates
#COPY cmd/web-api/static /static

WORKDIR ./cmd/web-api

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
