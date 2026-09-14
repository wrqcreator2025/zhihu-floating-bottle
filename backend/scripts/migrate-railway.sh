#!/bin/sh
set -eu
: "${MYSQL_URL:?Railway MYSQL_URL is required to run migrations}"
exec /migrate -path=/migrations -database="$MYSQL_URL" up
