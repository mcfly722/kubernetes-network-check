FROM golang:1.13.15 AS golang
ADD . /app
WORKDIR /app
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO11MODULE=on go build -a -o /main .

# we use ping from Debian, because alpine or busybox have very limited ping functional
FROM debian:stretch-slim AS debian
RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get install -y \
        iputils-ping
	
FROM alpine:3.12
COPY --from=golang /main /kubernetes-network-check
RUN chmod +x /kubernetes-network-check

RUN rm -rf /bin/ping
COPY --from=debian /bin/ping /bin/ping
RUN chmod +x /bin/ping