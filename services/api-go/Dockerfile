FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server \
    && CGO_ENABLED=0 go build -o /out/worker ./cmd/worker \
    && CGO_ENABLED=0 go build -o /out/seed ./cmd/seed

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/server /usr/local/bin/server
COPY --from=build /out/worker /usr/local/bin/worker
COPY --from=build /out/seed /usr/local/bin/seed
USER nobody
ENTRYPOINT ["server"]
