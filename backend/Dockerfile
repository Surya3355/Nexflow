# Nexflow API.
# Build stage compiles a static binary; migrations are embedded and run at startup.
FROM golang:1.27.4 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/nexflow ./cmd/nexflow
# An empty uploads folder owned by the non-root user, so the volume is writable.
RUN mkdir -p /out/uploads

# Test stage: `docker compose run --rm test` runs the whole Go suite against the compose database.
# The backup scripts come from the "infra" build context (docker-compose.yml) so the
# backup retention check (acceptance 19) can run them at /infra/backup.
FROM golang:1.27.4 AS test
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY --from=infra backup /infra/backup
COPY . .
CMD ["go", "test", "-count=1", "./..."]

# Runs as the distroless "nonroot" user (uid 65532), not as root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/nexflow /nexflow
COPY --from=build --chown=65532:65532 /out/uploads /data/uploads
VOLUME ["/data/uploads"]
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/nexflow"]
