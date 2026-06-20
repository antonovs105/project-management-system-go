#!/usr/bin/env sh
set -eu

PROGO_REPO=${PROGO_REPO:-antonovs105/project-management-system-go}
PROGO_REF=${PROGO_REF:-main}
APP_DIR=${APP_DIR:-/opt/progo/app}
CONFIG_FILE=${CONFIG_FILE:-"$APP_DIR/progo.yml"}
ENV_FILE=${ENV_FILE:-"$APP_DIR/.env"}
ARCHIVE_URL=${PROGO_ARCHIVE_URL:-"https://github.com/$PROGO_REPO/archive/$PROGO_REF.tar.gz"}
IMAGE_PREFIX=${IMAGE_PREFIX:-"ghcr.io/$(printf '%s' "$PROGO_REPO" | tr '[:upper:]' '[:lower:]')"}
IMAGE_TAG=${IMAGE_TAG:-main}
BACKEND_IMAGE=${BACKEND_IMAGE:-"$IMAGE_PREFIX/backend:$IMAGE_TAG"}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
    return
  fi
  sudo "$@"
}

compose_available() {
  docker compose version >/dev/null 2>&1
}

ensure_app_dir() {
  as_root mkdir -p "$APP_DIR"
  if [ "$(id -u)" -ne 0 ]; then
    as_root chown "$(id -u):$(id -g)" "$APP_DIR"
  fi
  mkdir -p "$APP_DIR"
}

download_assets() {
  tmpdir=$(mktemp -d)
  trap 'rm -rf "$tmpdir"' EXIT HUP INT TERM

  echo "downloading deploy assets from $ARCHIVE_URL"
  curl -fsSL "$ARCHIVE_URL" -o "$tmpdir/source.tar.gz"
  tar -xzf "$tmpdir/source.tar.gz" -C "$tmpdir"
  source_dir=$(find "$tmpdir" -mindepth 1 -maxdepth 1 -type d | head -n 1)
  if [ -z "$source_dir" ] || [ ! -d "$source_dir/deploy" ] || [ ! -d "$source_dir/migrations" ]; then
    echo "downloaded archive does not contain deploy assets" >&2
    exit 1
  fi

  rm -rf "$APP_DIR/deploy" "$APP_DIR/migrations"
  cp -R "$source_dir/deploy" "$APP_DIR/deploy"
  cp -R "$source_dir/migrations" "$APP_DIR/migrations"
  chmod +x "$APP_DIR/deploy/bluegreen-deploy.sh"
}

run_pmsctl() {
  docker run --rm \
    -u "$(id -u):$(id -g)" \
    -v "$APP_DIR:/work" \
    "$BACKEND_IMAGE" \
    /app/pmsctl "$@"
}

run_pmsctl_interactive() {
  if [ ! -r /dev/tty ]; then
    echo "interactive config requires a terminal; run this installer from an interactive shell" >&2
    exit 1
  fi
  docker run --rm -i \
    -u "$(id -u):$(id -g)" \
    -v "$APP_DIR:/work" \
    "$BACKEND_IMAGE" \
    /app/pmsctl "$@" < /dev/tty
}

init_config_once() {
  if [ -f "$CONFIG_FILE" ]; then
    echo "keeping existing runtime config: $CONFIG_FILE"
    return
  fi
  echo "creating runtime config: $CONFIG_FILE"
  docker pull "$BACKEND_IMAGE"
  run_pmsctl_interactive config init --output /work/progo.yml
}

init_env_once() {
  if [ -f "$ENV_FILE" ]; then
    echo "keeping existing deploy env: $ENV_FILE"
    return
  fi
  echo "creating deploy env: $ENV_FILE"
  run_pmsctl config export-env \
    --config /work/progo.yml \
    --output /work/.env \
    --image-prefix "$IMAGE_PREFIX" \
    --image-tag "$IMAGE_TAG"
}

need_cmd curl
need_cmd tar
need_cmd docker
need_cmd find
need_cmd cp
need_cmd rm
need_cmd caddy
if [ "$(id -u)" -ne 0 ]; then
  need_cmd sudo
fi
if ! compose_available; then
  echo "missing Docker Compose plugin: docker compose" >&2
  exit 1
fi

ensure_app_dir
download_assets
init_config_once
init_env_once

echo "running blue-green deploy"
APP_DIR="$APP_DIR" \
ENV_FILE="$ENV_FILE" \
IMAGE_PREFIX="$IMAGE_PREFIX" \
IMAGE_TAG="$IMAGE_TAG" \
"$APP_DIR/deploy/bluegreen-deploy.sh"
