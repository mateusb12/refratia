#!/usr/bin/env bash
# Starts ISOLATED local Postgres + Garage for tests/benchmark and writes
# testinfra/test.env. Never touches the developer .env, production DB or buckets.
# Usage: bash backend/testinfra/up.sh   then   set -a; . backend/testinfra/test.env; set +a
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
PG=refratia-test-pg
GA=refratia-test-garage
PG_PORT=${REFRATIA_TEST_PG_PORT:-55432}
GA_PORT=${REFRATIA_TEST_GARAGE_PORT:-53900}
BUCKET=refratia-test

if ! docker ps --format '{{.Names}}' | grep -qx "$PG"; then
  docker rm -f "$PG" >/dev/null 2>&1 || true
  docker run -d --name "$PG" -e POSTGRES_USER=refratia -e POSTGRES_PASSWORD=refratia \
    -e POSTGRES_DB=postgres -p 127.0.0.1:$PG_PORT:5432 postgres:17-alpine >/dev/null
fi

if ! docker ps --format '{{.Names}}' | grep -qx "$GA"; then
  docker rm -f "$GA" >/dev/null 2>&1 || true
  cat > "$here/garage.toml" <<EOF
metadata_dir = "/var/lib/garage/meta"
data_dir = "/var/lib/garage/data"
db_engine = "sqlite"
replication_factor = 1
rpc_bind_addr = "[::]:3901"
rpc_public_addr = "127.0.0.1:3901"
rpc_secret = "$(openssl rand -hex 32)"
[s3_api]
s3_region = "garage"
api_bind_addr = "[::]:3900"
root_domain = ".s3.garage.localhost"
[admin]
api_bind_addr = "[::]:3903"
EOF
  docker run -d --name "$GA" -p 127.0.0.1:$GA_PORT:3900 \
    -v "$here/garage.toml:/etc/garage.toml:ro" dxflrs/garage:v1.1.0 >/dev/null
  sleep 2
  g() { docker exec "$GA" /garage "$@"; }
  node=$(g node id -q | cut -d@ -f1)
  g layout assign -z dc1 -c 1G "$node" >/dev/null
  g layout apply --version 1 >/dev/null
  g bucket create "$BUCKET" >/dev/null
  keyinfo=$(g key create refratia-test-key)
  key_id=$(echo "$keyinfo" | awk '/Key ID/{print $3}')
  secret=$(echo "$keyinfo" | awk '/Secret key/{print $3}')
  g bucket allow --read --write --owner "$BUCKET" --key refratia-test-key >/dev/null
  echo "$key_id $secret" > "$here/.garage-key"
fi
read -r key_id secret < "$here/.garage-key"
# Separate buckets: refratia-test (dev/persistence tests), refratia-bench (benchmark, wiped per run).
for b in refratia-test refratia-bench; do
  docker exec "$GA" /garage bucket info "$b" >/dev/null 2>&1 || docker exec "$GA" /garage bucket create "$b" >/dev/null
  docker exec "$GA" /garage bucket allow --read --write --owner "$b" --key refratia-test-key >/dev/null
done

for _ in $(seq 30); do docker exec "$PG" pg_isready -U refratia >/dev/null 2>&1 && break; sleep 1; done

cat > "$here/test.env" <<EOF
REFRATIA_NO_DOTENV=1
OMP_THREAD_LIMIT=1
DATABASE_URL=postgres://refratia:refratia@127.0.0.1:$PG_PORT/refratia_test?sslmode=disable
POSTGRES_HOST=127.0.0.1
POSTGRES_PORT=$PG_PORT
POSTGRES_DB=refratia_test
POSTGRES_USER=refratia
POSTGRES_PASSWORD=refratia
POSTGRES_ADMIN_DB=postgres
POSTGRES_SSLMODE=disable
FILE_STORAGE_DRIVER=garage
GARAGE_S3_API_URL=http://127.0.0.1:$GA_PORT
GARAGE_S3_PUBLIC_API_URL=http://127.0.0.1:$GA_PORT
FILE_STORAGE_ENDPOINT_URL=http://127.0.0.1:$GA_PORT
FILE_STORAGE_PUBLIC_ENDPOINT_URL=http://127.0.0.1:$GA_PORT
AWS_ENDPOINT_URL_S3=http://127.0.0.1:$GA_PORT
AWS_PUBLIC_ENDPOINT_URL_S3=http://127.0.0.1:$GA_PORT
AWS_REGION=garage
AWS_ACCESS_KEY_ID=$key_id
AWS_SECRET_ACCESS_KEY=$secret
AWS_EC2_METADATA_DISABLED=true
BUCKET_NAME=$BUCKET
OPENAI_API_KEY=
CASE_DELETE_TOKEN=test-only
EOF
echo "isolated infra ready: $here/test.env"
