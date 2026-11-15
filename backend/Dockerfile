# Ubuntu 24.04 ships the same poppler (24.02), tesseract (5.3.4) and
# leptonica (1.82) the benchmark and its ground truth were built on. Alpine
# (musl, leptonica 1.84) read 14 Pentacam fields differently on the same files
# and ran OCR ~2x slower. Build and runtime share the base: the server links
# libtesseract (cgo).
FROM ubuntu:24.04 AS build
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
       gcc libc6-dev pkg-config libtesseract-dev libleptonica-dev ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=golang:1.23 /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:$PATH
WORKDIR /src
COPY go.mod ./
COPY go.sum ./
COPY . .
RUN go build -o /server ./cmd/server

FROM ubuntu:24.04
# wget: compose healthcheck. ca-certificates: TLS to object storage.
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
       poppler-utils tesseract-ocr tesseract-ocr-eng ca-certificates wget \
    && rm -rf /var/lib/apt/lists/*
# OCR concurrency is bounded in Go (pentacam readerConcurrency etc.) assuming
# each tesseract process is single-threaded; without this every process also
# spawns OpenMP threads and oversubscribes the CPU.
ENV OMP_THREAD_LIMIT=1
COPY --from=build /server /server
EXPOSE 3000
CMD ["/server"]
