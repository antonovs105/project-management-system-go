#!/usr/bin/env sh
set -eu

APP_DIR=${APP_DIR:-/opt/progo/app}
ENV_FILE=${ENV_FILE:-"$APP_DIR/.env"}
COMPOSE_FILE=${COMPOSE_FILE:-"$APP_DIR/deploy/docker-compose.bluegreen.yml"}
STATE_FILE=${STATE_FILE:-"$APP_DIR/.active-color"}
BACKUP_PATH=${1:-${BACKUP_PATH:-}}
BACKUP_AGE_IDENTITY=${BACKUP_AGE_IDENTITY:-}
SKIP_PRE_RESTORE_BACKUP=${SKIP_PRE_RESTORE_BACKUP:-false}
BACKUP_ATTACHMENT_SERVICE=${BACKUP_ATTACHMENT_SERVICE:-}
RESTORE_STOP_SERVICES=${RESTORE_STOP_SERVICES:-"backend-blue backend-worker-blue frontend-blue backend-green backend-worker-green frontend-green"}
RESTORE_START_SERVICES=${RESTORE_START_SERVICES:-}

env_value() {
  key=$1
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); sub(/\r$/, ""); print; exit }' "$ENV_FILE"
}

INSTANCE_NAME=${INSTANCE_NAME:-$(env_value INSTANCE_NAME)}
POSTGRES_USER=${POSTGRES_USER:-$(env_value POSTGRES_USER)}
POSTGRES_USER=${POSTGRES_USER:-postgres}
POSTGRES_DB=${POSTGRES_DB:-$(env_value POSTGRES_DB)}
POSTGRES_DB=${POSTGRES_DB:-pms}

compose() {
  docker compose --project-directory "$APP_DIR" --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

if [ -z "$BACKUP_PATH" ] || [ ! -d "$BACKUP_PATH" ]; then
  echo "usage: restore-backup.sh /path/to/backup-directory" >&2
  exit 1
fi
if [ -z "$INSTANCE_NAME" ]; then
  echo "INSTANCE_NAME is required" >&2
  exit 1
fi
if [ "${RESTORE_CONFIRM:-}" != "$INSTANCE_NAME" ]; then
  echo "refusing destructive restore; set RESTORE_CONFIRM=$INSTANCE_NAME" >&2
  exit 1
fi
if [ ! -f "$BACKUP_PATH/SHA256SUMS" ] || [ ! -f "$BACKUP_PATH/metadata.env" ]; then
  echo "backup is missing SHA256SUMS or metadata.env" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$BACKUP_PATH" && sha256sum -c SHA256SUMS)
elif command -v shasum >/dev/null 2>&1; then
  (cd "$BACKUP_PATH" && shasum -a 256 -c SHA256SUMS)
else
  echo "missing required restore command: sha256sum or shasum" >&2
  exit 1
fi

temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM
if [ -f "$BACKUP_PATH/database.dump.age" ]; then
  if [ -z "$BACKUP_AGE_IDENTITY" ] || ! command -v age >/dev/null 2>&1; then
    echo "encrypted backup requires age and BACKUP_AGE_IDENTITY" >&2
    exit 1
  fi
  age -d -i "$BACKUP_AGE_IDENTITY" -o "$temporary/database.dump" "$BACKUP_PATH/database.dump.age"
  age -d -i "$BACKUP_AGE_IDENTITY" -o "$temporary/attachments.tar.gz" "$BACKUP_PATH/attachments.tar.gz.age"
else
  cp "$BACKUP_PATH/database.dump" "$temporary/database.dump"
  cp "$BACKUP_PATH/attachments.tar.gz" "$temporary/attachments.tar.gz"
fi

compose exec -T db pg_restore --list < "$temporary/database.dump" >/dev/null
tar -tzf "$temporary/attachments.tar.gz" >/dev/null

active=blue
if [ -f "$STATE_FILE" ]; then
  candidate=$(tr -d '[:space:]' < "$STATE_FILE")
  if [ "$candidate" = "blue" ] || [ "$candidate" = "green" ]; then
    active=$candidate
  fi
fi
attachment_service=${BACKUP_ATTACHMENT_SERVICE:-"backend-$active"}
RESTORE_START_SERVICES=${RESTORE_START_SERVICES:-"backend-$active backend-worker-$active frontend-$active"}

if [ "$SKIP_PRE_RESTORE_BACKUP" != "true" ]; then
  echo "creating mandatory pre-restore safety backup"
  APP_DIR="$APP_DIR" ENV_FILE="$ENV_FILE" COMPOSE_FILE="$COMPOSE_FILE" \
    BACKUP_ATTACHMENT_SERVICE="$attachment_service" "$APP_DIR/deploy/backup.sh"
fi

echo "stopping application services for restore"
# Operator-provided service lists are intentionally word-split into Compose arguments.
# shellcheck disable=SC2086
compose stop $RESTORE_STOP_SERVICES >/dev/null 2>&1 || true
compose up -d --wait db redis

echo "restoring PostgreSQL database"
compose exec -T db dropdb --force --if-exists -U "$POSTGRES_USER" "$POSTGRES_DB"
compose exec -T db createdb -U "$POSTGRES_USER" "$POSTGRES_DB"
compose exec -T db pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --no-privileges < "$temporary/database.dump"

echo "restoring attachment objects"
compose run -T --rm --no-deps --entrypoint sh "$attachment_service" -c \
  'rm -rf /data/attachments/* /data/attachments/.[!.]* /data/attachments/..?*; tar -C /data/attachments -xzf -' \
  < "$temporary/attachments.tar.gz"

compose run --rm --no-deps migrations
# shellcheck disable=SC2086
compose up -d $RESTORE_START_SERVICES
echo "restore complete; verify local and public readiness before resuming operations"
