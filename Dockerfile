# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.2

FROM golang:${GO_VERSION}-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/neo-line ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=build /out/neo-line /app/neo-line

ENV GIN_MODE=release

EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/app/neo-line"]
