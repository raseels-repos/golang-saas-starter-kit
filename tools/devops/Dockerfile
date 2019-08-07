FROM golang:1.12.6-alpine3.9 AS builder

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

COPY internal ./internal
COPY tools/devops ./tools/devops

WORKDIR ./tools/devops

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix nocgo -o /go/bin/devops .

FROM scratch

COPY --from=builder /go/bin/devops /go/bin/devops

ENTRYPOINT ["/go/bin/devops"]
