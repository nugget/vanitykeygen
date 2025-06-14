FROM alpine:latest AS builder

RUN apk update && apk upgrade && apk add git make go

WORKDIR /build

COPY . .

RUN go mod download
RUN go mod verify

RUN make vkgstatic

FROM alpine:latest
COPY --from=builder /build/vkg-static-build /bin/vkg
RUN mkdir -p /vkgdata/keys
RUN mkdir -p /vkgdata/logs

EXPOSE 8080

CMD ["/bin/vkg", "server", "-l", "/vkgdata/logs/matchfile.log"]
