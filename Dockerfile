# Build
FROM golang:1.27-alpine AS build

RUN apk add --no-cache ca-certificates

WORKDIR /src
COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/k8s-scout ./cmd/k8s-scout

# Run
FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/k8s-scout /usr/local/bin/k8s-scout

ENV PORT=8080
EXPOSE 8080

USER 65532:65532
ENTRYPOINT ["k8s-scout"]
