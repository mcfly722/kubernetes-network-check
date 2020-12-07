FROM golang:1.14 AS golang
ADD . /app
WORKDIR /app
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO11MODULE=on go build -a -o /main .

FROM alpine:3.12 as kubectl
ARG K8S_VERSION=v1.14.3
RUN set -x                  && \
    apk --update upgrade    && \
    apk add ca-certificates && \
    rm -rf /var/cache/apk/* && \
    wget -O /kubectl https://storage.googleapis.com/kubernetes-release/release/$K8S_VERSION/bin/linux/amd64/kubectl && \
    chmod +x /kubectl
	
FROM alpine:3.12
COPY --from=kubectl /kubectl /kubectl
COPY --from=golang /main /kubernetes-network-check
RUN chmod +x /kubectl
RUN chmod +x /kubernetes-network-check