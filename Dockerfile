# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25
ARG TARGET_CMD=api

FROM golang:${GO_VERSION}-alpine AS build
ARG TARGET_CMD
WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${TARGET_CMD}

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/app /app/app

ENV GO_BACKEND_ADDR=:9000
EXPOSE 9000
ENTRYPOINT ["/app/app"]
