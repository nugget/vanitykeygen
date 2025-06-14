FROM golang:1-alpine AS builder

WORKDIR /build

COPY . .

RUN go mod download
RUN go mod verify

RUN go build -o vkg cmd/vkg/main.go

FROM alpine:latest

WORKDIR /

COPY --from=builder /build/vkg /bin/vkg

CMD ["/bin/vkg", "server"]
