FROM golang:alpine AS builder

ARG OCI_IMAGE_VERSION="dev"

RUN apk add --no-cache git

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-X 'main.gitVersion=${OCI_IMAGE_VERSION}'" -o vkg ./cmd/vkg

FROM alpine:latest
COPY --from=builder /build/vkg /bin/vkg
RUN mkdir -p /vkgdata

EXPOSE 8080

CMD ["/bin/vkg", "server", "-d", "/vkgdata/vkg.db"]
