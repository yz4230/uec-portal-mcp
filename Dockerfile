# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build

WORKDIR /src

ARG TARGETOS
ARG TARGETARCH

ENV CGO_ENABLED=0

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS="${TARGETOS:-linux}" GOARCH="${TARGETARCH:-amd64}" \
    go build -trimpath -ldflags="-s -w" -o /out/uec-portal-mcp .

FROM gcr.io/distroless/static-debian12:nonroot AS runtime

WORKDIR /

COPY --from=build /out/uec-portal-mcp /uec-portal-mcp

ENV PORT=8080

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/uec-portal-mcp"]
CMD ["serve", "--http"]
