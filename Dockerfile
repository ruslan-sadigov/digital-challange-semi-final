FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /agilli ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /agilli /agilli
EXPOSE 8080
ENTRYPOINT ["/agilli"]
