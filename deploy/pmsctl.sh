#!/usr/bin/env sh
set -eu

APP_DIR=${APP_DIR:-/opt/progo/app}
ENV_FILE=${ENV_FILE:-"$APP_DIR/.env"}
STATE_FILE=${STATE_FILE:-"$APP_DIR/.active-color"}

if [ ! -f "$ENV_FILE" ]; then
  echo "missing env file: $ENV_FILE" >&2
  exit 1
fi

if [ ! -f "$STATE_FILE" ]; then
  echo "missing active deployment state: $STATE_FILE" >&2
  exit 1
fi

env_value() {
  key=$1
  awk -F= -v key="$key" '$1 == key {
    sub(/^[^=]*=/, "")
    sub(/\r$/, "")
    print
    exit
  }' "$ENV_FILE"
}

INSTANCE_NAME=${INSTANCE_NAME:-$(env_value INSTANCE_NAME)}
COLOR=$(tr -d '[:space:]' < "$STATE_FILE")

if [ -z "$INSTANCE_NAME" ]; then
  echo "INSTANCE_NAME is required in $ENV_FILE" >&2
  exit 1
fi

case "$COLOR" in
  blue | green)
    ;;
  *)
    echo "invalid active deployment color: $COLOR" >&2
    exit 1
    ;;
esac

CONTAINER_NAME=${PMSCTL_CONTAINER:-"$INSTANCE_NAME-$COLOR-backend"}

if ! docker ps --format '{{.Names}}' | grep -qx "$CONTAINER_NAME"; then
  echo "active backend container is not running: $CONTAINER_NAME" >&2
  exit 1
fi

exec docker exec -i "$CONTAINER_NAME" /app/pmsctl "$@"
