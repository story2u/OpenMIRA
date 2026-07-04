# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25
ARG TARGET_CMD=api

FROM golang:${GO_VERSION}-alpine AS build
ARG TARGET_CMD
WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${TARGET_CMD}

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/app /app/app

ENV ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/app/app"]
