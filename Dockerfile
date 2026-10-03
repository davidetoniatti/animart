FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/animart .

FROM alpine:3
RUN apk add --no-cache ffmpeg ca-certificates tzdata
COPY --from=build /out/animart /usr/local/bin/animart
USER 1000:1000
ENTRYPOINT ["animart"]
CMD ["-music", "/music", "-cache", "/data/cache.db"]
