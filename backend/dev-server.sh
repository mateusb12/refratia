#!/bin/sh

set -eu

backend_binary=/tmp/refratia-backend
last_attempted_source_checksum=""
running_process_id=""

calculate_source_checksum() {
	find . -type f \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) -print \
		| sort \
		| xargs sha256sum \
		| sha256sum \
		| cut -d ' ' -f 1
}

stop_backend() {
	if [ -n "$running_process_id" ] && kill -0 "$running_process_id" 2>/dev/null; then
		kill "$running_process_id" 2>/dev/null || true
		wait "$running_process_id" 2>/dev/null || true
	fi
	running_process_id=""
}

cleanup() {
	stop_backend
}

trap cleanup INT TERM EXIT

while true; do
	source_checksum=$(calculate_source_checksum)

	if [ "$source_checksum" != "$last_attempted_source_checksum" ]; then
		last_attempted_source_checksum="$source_checksum"

		echo "[refratia] Compilando backend..."
		if go build -o "$backend_binary" ./cmd/server; then
			echo "[refratia] Backend compilado; iniciando servidor."
			stop_backend
			"$backend_binary" &
			running_process_id=$!
		else
			echo "[refratia] Falha na compilação; aguardando próxima alteração."
		fi
	fi

	if [ -n "$running_process_id" ] && ! kill -0 "$running_process_id" 2>/dev/null; then
		wait "$running_process_id" 2>/dev/null || true
		running_process_id=""
	fi

	sleep 1
done
