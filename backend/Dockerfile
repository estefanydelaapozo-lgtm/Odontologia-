FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sigo-dental .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget \
    && addgroup -S -g 10001 sigo \
    && adduser -S -D -H -u 10001 -G sigo sigo \
    && mkdir -p /app/var/clinical \
    && chown -R sigo:sigo /app
WORKDIR /app
COPY --from=build --chown=sigo:sigo /out/sigo-dental /app/sigo-dental
USER sigo
EXPOSE 3000
ENTRYPOINT ["/app/sigo-dental"]
