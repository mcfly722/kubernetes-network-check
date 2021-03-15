FROM golang:1.13.15 AS golang
ADD . /app
WORKDIR /app
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO11MODULE=on go build -a -o /main .
	
FROM alpine:3.12
COPY --from=golang /main /kubernetes-network-check
RUN apk add --no-cache ca-certificates && \
    update-ca-certificates && \
    apk add --no-cache --virtual iputils && \
    chmod +x /kubernetes-network-check