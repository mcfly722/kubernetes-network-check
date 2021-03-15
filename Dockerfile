FROM golang:1.13.15 AS golang
ADD . /app
WORKDIR /app
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO11MODULE=on go build -a -o /main .
	
FROM alpine:3.12
COPY --from=golang /main /kubernetes-network-check
RUN apk add --no-cache ca-certificates && \
    update-ca-certificates
RUN apk add --no-cache --virtual .build-deps wget gnupg tar iputils && \
RUN chmod +x /kubernetes-network-check