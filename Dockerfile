# syntax=docker/dockerfile:1

FROM golang:1.26.3-bookworm AS go-build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=0.0.0-dev
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/wa-dd-api ./cmd/wa-dd-api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/wa-dd ./cmd/wa-dd

FROM golang:1.26.3-bookworm AS goose-build
WORKDIR /src/tools/goose

COPY tools/goose/go.mod tools/goose/go.sum ./
RUN go mod download

COPY tools/goose ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/goose github.com/pressly/goose/v3/cmd/goose

FROM gcr.io/distroless/static-debian12 AS api
WORKDIR /app
COPY --from=go-build /out/wa-dd-api /usr/local/bin/wa-dd-api
COPY db/migrations /app/db/migrations
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["wa-dd-api"]

FROM gcr.io/distroless/static-debian12 AS cli
WORKDIR /app
COPY --from=go-build /out/wa-dd /usr/local/bin/wa-dd
COPY config /app/config
COPY db/migrations /app/db/migrations
USER nonroot:nonroot
ENTRYPOINT ["wa-dd"]

FROM debian:bookworm-slim AS migrate
WORKDIR /app
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/*
COPY --from=goose-build /out/goose /usr/local/bin/goose
COPY db/migrations /app/db/migrations
RUN useradd --system --uid 65532 --home-dir /nonexistent --shell /usr/sbin/nologin nonroot
USER nonroot
CMD ["sh", "-c", "goose -dir /app/db/migrations postgres \"$DATABASE_URL\" up"]
