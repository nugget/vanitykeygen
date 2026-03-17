FROM golang:alpine AS builder

RUN apk add --no-cache git make

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN make vkgstatic

FROM alpine:latest
COPY --from=builder /build/vkg-static-build /bin/vkg
RUN mkdir -p /vkgdata

EXPOSE 8080

CMD ["/bin/vkg", "server", "-d", "/vkgdata/vkg.db"]
