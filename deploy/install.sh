#!/usr/bin/env sh
set -eu

PROGO_REPO=${PROGO_REPO:-antonovs105/project-management-system-go}
PROGO_REF=${PROGO_REF:-main}
APP_DIR=${APP_DIR:-/opt/progo/app}
CONFIG_FILE=${CONFIG_FILE:-"$APP_DIR/progo.yml"}
ENV_FILE=${ENV_FILE:-"$APP_DIR/.env"}
ARCHIVE_URL=${PROGO_ARCHIVE_URL:-"https://github.com/$PROGO_REPO/archive/$PROGO_REF.tar.gz"}
ARCHIVE_SHA256=${PROGO_ARCHIVE_SHA256:-}
ALLOW_UNVERIFIED_DOWNLOAD=${PROGO_ALLOW_UNVERIFIED_DOWNLOAD:-false}
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

verify_archive() {
  if [ -z "$ARCHIVE_SHA256" ]; then
    if [ "$ALLOW_UNVERIFIED_DOWNLOAD" = "true" ]; then
      echo "warning: installing an archive without integrity verification" >&2
      return
    fi
    echo "PROGO_ARCHIVE_SHA256 is required; set PROGO_ALLOW_UNVERIFIED_DOWNLOAD=true only for local testing" >&2
    exit 1
  fi
  case "$ARCHIVE_SHA256" in
    *[!0-9A-Fa-f]*|'')
      echo "PROGO_ARCHIVE_SHA256 must be a hexadecimal SHA-256 digest" >&2
      exit 1
      ;;
  esac
  if [ "${#ARCHIVE_SHA256}" -ne 64 ]; then
    echo "PROGO_ARCHIVE_SHA256 must contain exactly 64 hexadecimal characters" >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$tmpdir/source.tar.gz")
  else
    actual=$(shasum -a 256 "$tmpdir/source.tar.gz")
  fi
  actual=${actual%% *}
  if [ "$(printf '%s' "$actual" | tr '[:upper:]' '[:lower:]')" != "$(printf '%s' "$ARCHIVE_SHA256" | tr '[:upper:]' '[:lower:]')" ]; then
    echo "downloaded archive SHA-256 does not match PROGO_ARCHIVE_SHA256" >&2
    exit 1
  fi
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
  verify_archive
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
  chmod +x "$APP_DIR/deploy/backup.sh"
  chmod +x "$APP_DIR/deploy/restore-backup.sh"
  if [ -f "$APP_DIR/deploy/pmsctl.sh" ]; then
    chmod +x "$APP_DIR/deploy/pmsctl.sh"
  fi
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
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  echo "missing required command: sha256sum or shasum" >&2
  exit 1
fi
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

echo
echo "deployment is healthy"
echo "new instance: create the first owner from the active backend container"
echo "  cd $APP_DIR"
echo "  read -r -s OWNER_PASSWORD"
echo "  printf '%s\\n' \"\$OWNER_PASSWORD\" | ./deploy/pmsctl.sh owner create --username owner --email owner@example.test --password-stdin"
echo "  unset OWNER_PASSWORD"
echo "existing instance recovery runbook: $APP_DIR/deploy/OWNER_RECOVERY.md"
