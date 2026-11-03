# ---- build -------------------------------------------------------------
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
# Static binary, no cgo, stripped: runs on a distroless base with no shell.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/aegis ./cmd/aegis

# ---- run ---------------------------------------------------------------
# distroless/static: no shell, no package manager - nothing for an attacker
# who compromises the container to work with.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/aegis /aegis
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/aegis"]
CMD ["serve", "--repo", "/repo", "--source", "/data", "--addr", ":8080"]
