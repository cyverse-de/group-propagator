FROM golang:1.21 as build-root

WORKDIR /go/src/github.com/cyverse-de/group-propagator

COPY go.mod .
COPY go.sum .

COPY . .

ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

RUN go build --buildvcs=false .
RUN go clean -cache -modcache
RUN cp ./group-propagator /bin/group-propagator

ENTRYPOINT ["group-propagator"]

EXPOSE 60000
