#!/bin/sh
set -eu
: "${MYSQL_URL:?Railway MYSQL_URL is required to run migrations}"

# Railway provides mysql://user:password@host:port/database, while
# golang-migrate's MySQL driver expects the go-sql-driver form tcp(host:port).
case "$MYSQL_URL" in
    mysql://*@tcp\(*\)/*)
        database_url="$MYSQL_URL"
        ;;
    mysql://*@*/*)
        credentials=${MYSQL_URL%%@*}
        host_and_database=${MYSQL_URL#*@}
        host=${host_and_database%%/*}
        database=${host_and_database#*/}
        if [ "$host" = "$host_and_database" ] || [ -z "$host" ] || [ -z "$database" ]; then
            echo "MYSQL_URL must include a host and database" >&2
            exit 1
        fi
        database_url="${credentials}@tcp(${host})/${database}"
        ;;
    *)
        echo "MYSQL_URL must use the mysql:// scheme" >&2
        exit 1
        ;;
esac

exec /migrate -path=/migrations -database="$database_url" up
