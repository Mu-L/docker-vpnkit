# syntax=docker/dockerfile:1
FROM linuxkit/alpine:7f3944798557de5518a56e3437d7ed982701f224 AS mirror

RUN apk add --no-cache go musl-dev build-base

ENV GOPATH=/go
COPY . /go/src/github.com/moby/vpnkit
RUN go-compile.sh /go/src/github.com/moby/vpnkit/go/cmd/vpnkit-forwarder

FROM scratch
COPY --link --from=mirror /go/bin/vpnkit-forwarder /usr/bin/vpnkit-forwarder
CMD ["/usr/bin/vpnkit-forwarder"]
