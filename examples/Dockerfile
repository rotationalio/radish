FROM golang:1.14 AS builder

# This assumes that you're building the docker file from the root of the radish repo:
# docker build -t kansaslabs/turnip:latest -f examples/Dockerfile .
WORKDIR /go/src/github.com/kansaslabs/radish/
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o turnip ./cmd/turnip/

FROM alpine:latest

LABEL maintainer="Kansas Labs <kansaslabs@gmail.com>"
LABEL description="Turnip is a demo/benchmark implementation of a radish task queue service."

RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /go/src/github.com/kansaslabs/radish/turnip .

ENTRYPOINT [ "./turnip", "serve" ]
