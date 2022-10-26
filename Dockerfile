# syntax=docker/dockerfile:1

## Build
FROM golang:1.19-buster AS build

WORKDIR /app

COPY . .
RUN cd cmd/mesh && CGO_ENABLED=0 go build -o /component

## Deploy
FROM gcr.io/distroless/static-debian11

WORKDIR /

COPY --from=build /component /component

ENTRYPOINT ["/component"]