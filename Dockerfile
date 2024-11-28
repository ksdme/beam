# Build
FROM golang:1.23-alpine3.20 AS build

WORKDIR /app
COPY . /app
RUN CGO=0 go build -o beam cmd/main.go

# Runtime
FROM alpine:3.20
COPY --from=build /app/beam /usr/local/bin
