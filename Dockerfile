FROM --platform=$BUILDPLATFORM golang:1.27 AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/valet ./cmd

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/valet /valet

EXPOSE 8001

ENTRYPOINT ["/valet"]
