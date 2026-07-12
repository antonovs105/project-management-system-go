#!/usr/bin/env sh
set -eu

APP_DIR=${APP_DIR:-/opt/progo/app}
ENV_FILE=${ENV_FILE:-"$APP_DIR/.env"}
COMPOSE_FILE=${COMPOSE_FILE:-"$APP_DIR/deploy/docker-compose.bluegreen.yml"}
BACKUP_DIR=${BACKUP_DIR:-"$APP_DIR/backups"}
BACKUP_RETENTION_DAYS=${BACKUP_RETENTION_DAYS:-}
BACKUP_OFFSITE_DIR=${BACKUP_OFFSITE_DIR:-}
BACKUP_AGE_RECIPIENT=${BACKUP_AGE_RECIPIENT:-}
BACKUP_ATTACHMENT_SERVICE=${BACKUP_ATTACHMENT_SERVICE:-backend-blue}

env_value() {
  key=$1
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); sub(/\r$/, ""); print; exit }' "$ENV_FILE"
}

INSTANCE_NAME=${INSTANCE_NAME:-$(env_value INSTANCE_NAME)}
BACKUP_RETENTION_DAYS=${BACKUP_RETENTION_DAYS:-$(env_value BACKUP_RETENTION_DAYS)}
BACKUP_RETENTION_DAYS=${BACKUP_RETENTION_DAYS:-14}
BACKUP_OFFSITE_DIR=${BACKUP_OFFSITE_DIR:-$(env_value BACKUP_OFFSITE_DIR)}
BACKUP_AGE_RECIPIENT=${BACKUP_AGE_RECIPIENT:-$(env_value BACKUP_AGE_RECIPIENT)}
POSTGRES_USER=${POSTGRES_USER:-$(env_value POSTGRES_USER)}
POSTGRES_USER=${POSTGRES_USER:-postgres}
POSTGRES_DB=${POSTGRES_DB:-$(env_value POSTGRES_DB)}
POSTGRES_DB=${POSTGRES_DB:-pms}

compose() {
  docker compose --project-directory "$APP_DIR" --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required backup command: $1" >&2
    exit 1
  fi
}

case "$BACKUP_RETENTION_DAYS" in
  ''|*[!0-9]*)
    echo "BACKUP_RETENTION_DAYS must be a non-negative integer" >&2
    exit 1
    ;;
esac

if [ -z "$INSTANCE_NAME" ]; then
  echo "INSTANCE_NAME is required" >&2
  exit 1
fi

require_command docker
require_command tar
if command -v sha256sum >/dev/null 2>&1; then
  SHA256_COMMAND=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  SHA256_COMMAND="shasum -a 256"
else
  echo "missing required backup command: sha256sum or shasum" >&2
  exit 1
fi
if [ -n "$BACKUP_AGE_RECIPIENT" ]; then
  require_command age
fi

mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR" >/dev/null 2>&1 || true
timestamp=$(date -u +%Y%m%dT%H%M%SZ)
target="$BACKUP_DIR/$INSTANCE_NAME-$timestamp"
temporary="$target.tmp"
umask 077
rm -rf "$temporary"
mkdir "$temporary"
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

echo "creating PostgreSQL custom-format backup"
compose exec -T db pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc > "$temporary/database.dump"
compose exec -T db pg_restore --list < "$temporary/database.dump" >/dev/null

echo "creating attachment object backup from $BACKUP_ATTACHMENT_SERVICE"
compose run -T --rm --no-deps --entrypoint sh "$BACKUP_ATTACHMENT_SERVICE" -c 'tar -C /data/attachments -czf - .' > "$temporary/attachments.tar.gz"
tar -tzf "$temporary/attachments.tar.gz" >/dev/null

cat > "$temporary/metadata.env" <<EOF
INSTANCE_NAME=$INSTANCE_NAME
CREATED_AT=$timestamp
POSTGRES_DB=$POSTGRES_DB
FORMAT_VERSION=1
ENCRYPTED=$([ -n "$BACKUP_AGE_RECIPIENT" ] && printf true || printf false)
EOF

if [ -n "$BACKUP_AGE_RECIPIENT" ]; then
  for file in database.dump attachments.tar.gz; do
    age -r "$BACKUP_AGE_RECIPIENT" -o "$temporary/$file.age" "$temporary/$file"
    rm -f "$temporary/$file"
  done
fi

(
  cd "$temporary"
  # shellcheck disable=SC2086
  $SHA256_COMMAND database.dump* attachments.tar.gz* metadata.env > SHA256SUMS
)

mv "$temporary" "$target"
trap - EXIT HUP INT TERM
echo "backup verified and committed: $target"

if [ -n "$BACKUP_OFFSITE_DIR" ]; then
  mkdir -p "$BACKUP_OFFSITE_DIR"
  chmod 700 "$BACKUP_OFFSITE_DIR" >/dev/null 2>&1 || true
  cp -R "$target" "$BACKUP_OFFSITE_DIR/"
  echo "backup copied off-host: $BACKUP_OFFSITE_DIR/$(basename "$target")"
fi

find "$BACKUP_DIR" -mindepth 1 -maxdepth 1 -type d -name "$INSTANCE_NAME-*" -mtime "+$BACKUP_RETENTION_DAYS" -exec rm -rf {} \;
