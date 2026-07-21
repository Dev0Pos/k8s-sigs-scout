# Build
FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/k8s-scout .

# Run
FROM alpine:3.22

RUN apk add --no-cache ca-certificates

COPY --from=build /out/k8s-scout /usr/local/bin/k8s-scout

ENV PORT=8080
EXPOSE 8080

USER nobody
ENTRYPOINT ["k8s-scout"]
