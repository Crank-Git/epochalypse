FROM golang:1.27.3-alpine@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY third_party ./third_party
ENV GOCACHE=/src/var/cache/go-build GOMODCACHE=/src/var/cache/go-mod GOPATH=/src/var/go GOTMPDIR=/src/var/tmp
RUN mkdir -p /src/var/tmp && go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
ARG EPOCHALYPSE_BUILD_COMMIT=unknown
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-X github.com/Crank-Git/epochalypse/internal/version.Commit=${EPOCHALYPSE_BUILD_COMMIT}" -o /epochalypse ./cmd/epochalypse

FROM alpine:3.22.2@sha256:4b7ce07002c69e8f3d704a9c5d6fd3053be500b7f1c69fc0d80990c2ad8dd412
WORKDIR /app
COPY --from=build /epochalypse /usr/local/bin/epochalypse
COPY config/local.example.json /app/config/local.json
RUN mkdir /state && chown 65534:65534 /state
ENTRYPOINT ["epochalypse"]
