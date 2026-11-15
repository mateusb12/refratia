FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY go.sum ./
COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache \
    poppler-utils \
    tesseract-ocr \
    tesseract-ocr-data-eng
# OCR concurrency is bounded in Go (pentacam readerConcurrency etc.) assuming
# each tesseract process is single-threaded; without this every process also
# spawns OpenMP threads and oversubscribes the CPU.
ENV OMP_THREAD_LIMIT=1
COPY --from=build /server /server
EXPOSE 3000
CMD ["/server"]
