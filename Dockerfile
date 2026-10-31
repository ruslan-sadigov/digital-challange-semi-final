FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /smartcity ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /smartcity /smartcity
EXPOSE 8080
ENTRYPOINT ["/smartcity"]
