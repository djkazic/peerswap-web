###
## Build PeerSwap and PeerSwap Web UI in a joint container 
###

FROM golang:1.22.2-bookworm AS builder

ARG TARGETOS
ARG TARGETARCH
ARG COMMIT

WORKDIR /app

COPY . .

RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} make -j1 install-lnd && \
    git clone https://github.com/ElementsProject/peerswap.git && \
    cd peerswap && \
    git checkout $COMMIT && \
    make -j1 lnd-release

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y supervisor ca-certificates && \
    mkdir -p /var/log/supervisor

COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY --from=builder /go/bin/* /bin/

RUN useradd -rm -s /bin/bash -u 1000 -U peerswap 
USER peerswap

EXPOSE 1984
EXPOSE 1985

CMD ["/usr/bin/supervisord"]
