FROM docker.io/library/golang:1.21 AS builder

WORKDIR /hostd-pin

# get dependencies
COPY go.mod go.sum ./
RUN go mod download

# copy source
COPY . .
# build
RUN go build -o bin/ -tags='netgo timetzdata' -trimpath -a -ldflags '-s -w -linkmode external -extldflags "-static"'  ./cmd/hpind

FROM docker.io/library/alpine:3

ENV PUID=0
ENV PGID=0

# copy binary and prepare data dir.
COPY --from=builder /hostd-pin/bin/* /usr/bin/
VOLUME [ "/data" ]

USER ${PUID}:${PGID}

ENTRYPOINT [ "hpind", "--config", "/data/config.yml" ]