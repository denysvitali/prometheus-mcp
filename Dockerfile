FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/prometheus-mcp .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/prometheus-mcp /usr/local/bin/prometheus-mcp
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/prometheus-mcp"]
CMD ["http", "--listen-address=:8080"]
