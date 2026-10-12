FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/model-notifier ./cmd/model-notifier \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/model-notifier /model-notifier
COPY config.yaml /etc/model-notifier/config.yaml
COPY --from=build --chown=65532:65532 /out/data /data
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s CMD ["/model-notifier", "-healthcheck"]
ENTRYPOINT ["/model-notifier"]
CMD ["-config", "/etc/model-notifier/config.yaml"]
