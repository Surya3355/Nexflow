# Nexflow API. Local use only (not production-hardened).
# Build stage compiles a static binary; migrations are embedded and run at startup.
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/nexflow ./cmd/nexflow

# Test stage: `docker compose run --rm test` runs the whole Go suite against the compose database.
FROM golang:1.25 AS test
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
CMD ["go", "test", "-count=1", "./..."]

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/nexflow /nexflow
VOLUME ["/data/uploads"]
EXPOSE 8080
ENTRYPOINT ["/nexflow"]
