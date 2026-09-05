FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO stays off: modernc.org/sqlite is pure Go, so the binary is static.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/butaca ./cmd/butaca

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 1000 butaca
COPY --from=build /out/butaca /usr/local/bin/butaca
USER butaca
ENV BUTACA_DATA_DIR=/data
VOLUME ["/data"]
ENTRYPOINT ["butaca"]
CMD ["serve"]
