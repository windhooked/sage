FROM golang:1.12-alpine as builder

RUN apk add --no-cache \
            bash \
            git \
            make \
            npm \
            openssl

ENV CGO_ENABLED=0

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION
RUN make build

FROM scratch

COPY --from=builder /src/sage /
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

WORKDIR /data
ENTRYPOINT ["/sage"]
CMD ["-server", "-rules=/data/ledger.rules", "-ledger=/data/ledger.journal", "-ofxclient=/data/ofxclient.ini"]
VOLUME ["/data"]
