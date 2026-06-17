#!/usr/bin/env sh
set -eu

APP_DIR=${APP_DIR:-/opt/progo/app}
ENV_FILE=${ENV_FILE:-"$APP_DIR/.env"}
COMPOSE_FILE=${COMPOSE_FILE:-"$APP_DIR/deploy/docker-compose.bluegreen.yml"}
STATE_FILE=${STATE_FILE:-"$APP_DIR/.active-color"}
TAG_FILE=${TAG_FILE:-"$APP_DIR/.active-image-tag"}

BLUE_BACKEND_PORT=${BLUE_BACKEND_PORT:-18080}
BLUE_FRONTEND_PORT=${BLUE_FRONTEND_PORT:-15173}
BLUE_BACKEND_WORKER_METRICS_PORT=${BLUE_BACKEND_WORKER_METRICS_PORT:-19091}
GREEN_BACKEND_PORT=${GREEN_BACKEND_PORT:-18081}
GREEN_FRONTEND_PORT=${GREEN_FRONTEND_PORT:-15174}
GREEN_BACKEND_WORKER_METRICS_PORT=${GREEN_BACKEND_WORKER_METRICS_PORT:-19092}

if [ ! -f "$ENV_FILE" ]; then
  echo "missing env file: $ENV_FILE" >&2
  exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "missing compose file: $COMPOSE_FILE" >&2
  exit 1
fi

env_value() {
  key=$1
  awk -F= -v key="$key" '$1 == key {
    sub(/^[^=]*=/, "")
    print
    exit
  }' "$ENV_FILE"
}

INSTANCE_NAME=${INSTANCE_NAME:-$(env_value INSTANCE_NAME)}
DOMAIN=${PROGO_DOMAIN:-$(env_value LOCAL_DOMAIN)}
PUBLIC_BASE_URL=${PUBLIC_BASE_URL:-$(env_value PUBLIC_BASE_URL)}
FEDERATION_NETWORK=${FEDERATION_NETWORK:-$(env_value FEDERATION_NETWORK)}
FEDERATION_NETWORK=${FEDERATION_NETWORK:-pms-federation}
MIGRATIONS_DIR=${MIGRATIONS_DIR:-"$APP_DIR/migrations"}

if [ -z "$INSTANCE_NAME" ]; then
  echo "INSTANCE_NAME is required in $ENV_FILE" >&2
  exit 1
fi

if [ -z "$DOMAIN" ]; then
  echo "LOCAL_DOMAIN is required in $ENV_FILE" >&2
  exit 1
fi

if [ -z "${BACKEND_IMAGE:-}" ] || [ -z "${FRONTEND_IMAGE:-}" ]; then
  if [ -z "${IMAGE_PREFIX:-}" ] || [ -z "${IMAGE_TAG:-}" ]; then
    echo "set BACKEND_IMAGE and FRONTEND_IMAGE, or set IMAGE_PREFIX and IMAGE_TAG" >&2
    exit 1
  fi
  BACKEND_IMAGE=${BACKEND_IMAGE:-"$IMAGE_PREFIX/backend:$IMAGE_TAG"}
  FRONTEND_IMAGE=${FRONTEND_IMAGE:-"$IMAGE_PREFIX/frontend:$IMAGE_TAG"}
fi

export INSTANCE_NAME
export BACKEND_IMAGE
export FRONTEND_IMAGE
export FEDERATION_NETWORK
export MIGRATIONS_DIR
export BLUE_BACKEND_PORT BLUE_FRONTEND_PORT BLUE_BACKEND_WORKER_METRICS_PORT
export GREEN_BACKEND_PORT GREEN_FRONTEND_PORT GREEN_BACKEND_WORKER_METRICS_PORT

compose() {
  docker compose --project-directory "$APP_DIR" --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

service_running() {
  name=$1
  docker ps --format '{{.Names}}' | grep -qx "$name"
}

port_in_use() {
  port=$1
  ss -ltn | awk '{print $4}' | grep -Eq "[:.]$port$"
}

detect_active_color() {
  if [ -f "$STATE_FILE" ]; then
    color=$(tr -d '[:space:]' < "$STATE_FILE")
    if [ "$color" = "blue" ] || [ "$color" = "green" ]; then
      echo "$color"
      return
    fi
  fi

  if service_running "$INSTANCE_NAME-blue-backend"; then
    echo blue
    return
  fi
  if service_running "$INSTANCE_NAME-green-backend"; then
    echo green
    return
  fi
  if service_running "$INSTANCE_NAME-backend" || service_running "$INSTANCE_NAME-frontend"; then
    echo legacy
    return
  fi
  echo none
}

ports_for_color() {
  color=$1
  if [ "$color" = "blue" ]; then
    BACKEND_PORT=$BLUE_BACKEND_PORT
    FRONTEND_PORT=$BLUE_FRONTEND_PORT
    WORKER_METRICS_PORT=$BLUE_BACKEND_WORKER_METRICS_PORT
  elif [ "$color" = "green" ]; then
    BACKEND_PORT=$GREEN_BACKEND_PORT
    FRONTEND_PORT=$GREEN_FRONTEND_PORT
    WORKER_METRICS_PORT=$GREEN_BACKEND_WORKER_METRICS_PORT
  else
    echo "invalid color: $color" >&2
    exit 1
  fi
  export BACKEND_PORT FRONTEND_PORT WORKER_METRICS_PORT
}

inactive_color_for() {
  active=$1
  case "$active" in
    blue)
      echo green
      ;;
    green)
      echo blue
      ;;
    legacy)
      if port_in_use "$GREEN_BACKEND_PORT" || port_in_use "$GREEN_FRONTEND_PORT"; then
        echo blue
      else
        echo green
      fi
      ;;
    none)
      if port_in_use "$BLUE_BACKEND_PORT" || port_in_use "$BLUE_FRONTEND_PORT"; then
        echo green
      else
        echo blue
      fi
      ;;
    *)
      echo "unknown active color: $active" >&2
      exit 1
      ;;
  esac
}

wait_for_url() {
  url=$1
  label=$2
  attempts=${3:-60}
  delay=${4:-2}
  i=1
  while [ "$i" -le "$attempts" ]; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      echo "$label is ready"
      return
    fi
    sleep "$delay"
    i=$((i + 1))
  done
  echo "$label did not become ready: $url" >&2
  exit 1
}

write_caddyfile() {
  backend_port=$1
  frontend_port=$2
  tmp=$(mktemp)
  cat > "$tmp" <<EOF
$DOMAIN {
  encode zstd gzip

  @backend_api {
    path /api* /auth* /.well-known/* /users* /tickets* /comments* /activities* /health /ready
  }
  handle @backend_api {
    reverse_proxy 127.0.0.1:$backend_port
  }

  @backend_public_posts {
    method POST
    path /register /login /webhooks/github
  }
  handle @backend_public_posts {
    reverse_proxy 127.0.0.1:$backend_port
  }

  @project_inbox {
    method POST
    path_regexp project_inbox ^/projects/[^/]+/inbox$
  }
  handle @project_inbox {
    reverse_proxy 127.0.0.1:$backend_port
  }

  @project_activitypub {
    path /projects/*
    header_regexp Accept (?i)application/(activity\+json|ld\+json|json)
  }
  handle @project_activitypub {
    reverse_proxy 127.0.0.1:$backend_port
  }

  handle {
    reverse_proxy 127.0.0.1:$frontend_port
  }
}
EOF
  sudo caddy fmt --overwrite "$tmp" >/dev/null
  caddy validate --config "$tmp" --adapter caddyfile >/dev/null
  sudo cp "$tmp" /etc/caddy/Caddyfile
  sudo chown root:root /etc/caddy/Caddyfile
  sudo chmod 644 /etc/caddy/Caddyfile
  sudo systemctl reload caddy
  rm -f "$tmp"
}

stop_color() {
  color=$1
  if [ "$color" = "blue" ] || [ "$color" = "green" ]; then
    compose stop "frontend-$color" "backend-$color" "backend-worker-$color" >/dev/null 2>&1 || true
    compose rm -f "frontend-$color" "backend-$color" "backend-worker-$color" >/dev/null 2>&1 || true
  fi
}

stop_legacy() {
  for name in "$INSTANCE_NAME-frontend" "$INSTANCE_NAME-backend" "$INSTANCE_NAME-backend-worker"; do
    if docker ps -a --format '{{.Names}}' | grep -qx "$name"; then
      docker stop "$name" >/dev/null 2>&1 || true
      docker rm "$name" >/dev/null 2>&1 || true
    fi
  done
}

echo "deploying backend image: $BACKEND_IMAGE"
echo "deploying frontend image: $FRONTEND_IMAGE"

docker network inspect "$FEDERATION_NETWORK" >/dev/null 2>&1 || docker network create "$FEDERATION_NETWORK" >/dev/null

active=$(detect_active_color)
inactive=$(inactive_color_for "$active")
ports_for_color "$inactive"

echo "active=$active inactive=$inactive backend_port=$BACKEND_PORT frontend_port=$FRONTEND_PORT"

stop_color "$inactive"

if [ "${SKIP_PULL:-false}" = "true" ]; then
  echo "SKIP_PULL=true; using images already present on the host"
else
  compose pull "backend-$inactive" "backend-worker-$inactive" "frontend-$inactive"
fi
compose up -d --wait db redis
compose run --rm --no-deps migrations
compose up -d "backend-$inactive" "backend-worker-$inactive"

wait_for_url "http://127.0.0.1:$BACKEND_PORT/ready" "backend-$inactive"

compose up -d "frontend-$inactive"
wait_for_url "http://127.0.0.1:$FRONTEND_PORT/health" "frontend-$inactive"

write_caddyfile "$BACKEND_PORT" "$FRONTEND_PORT"

if [ -n "$PUBLIC_BASE_URL" ]; then
  wait_for_url "$PUBLIC_BASE_URL/ready" "public backend"
fi

printf '%s\n' "$inactive" > "$STATE_FILE"
printf '%s\n' "${IMAGE_TAG:-$BACKEND_IMAGE}" > "$TAG_FILE"

if [ "$active" = "legacy" ]; then
  stop_legacy
else
  stop_color "$active"
fi

compose ps
echo "deploy_complete active=$inactive"
