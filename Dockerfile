FROM golang:1.25-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto
RUN go mod download
COPY . .
# assets (soul, mystery case, radio catalogue) are embedded — the binary is
# the whole product.
RUN CGO_ENABLED=0 go build -trimpath -o /ryolink ./cmd/ryolink

FROM alpine:latest
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 ryolink
COPY --from=build /ryolink /usr/local/bin/ryolink
RUN mkdir -p /data && chown ryolink /data
USER ryolink
WORKDIR /data
EXPOSE 2222 8090
ENTRYPOINT ["ryolink", "up"]
