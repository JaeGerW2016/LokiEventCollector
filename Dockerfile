FROM golang:1.14-buster as builder
ENV GO111MODULE on
WORKDIR /go/cache
COPY [ "go.mod","go.sum","./"]
RUN go mod download

WORKDIR /go/release
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o loki-event-collector .

FROM alpine:3.12
WORKDIR /
COPY --from=builder /go/release/loki-event-collector .
ENTRYPOINT ["/loki-event-collector"]