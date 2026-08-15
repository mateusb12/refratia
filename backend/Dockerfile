FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY go.sum ./
COPY main.go ./
RUN CGO_ENABLED=0 go build -o /server .

FROM alpine:3.20
COPY --from=build /server /server
EXPOSE 3000
CMD ["/server"]
