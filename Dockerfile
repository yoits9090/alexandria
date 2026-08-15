FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /alexandria ./cmd/alexandria

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /alexandria /alexandria
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/alexandria"]
