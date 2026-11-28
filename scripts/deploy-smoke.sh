#!/usr/bin/env bash
# Deploy smoke test: runs a Plantry image the way a server does and checks
# that it starts, answers, upgrades a real database, survives restarts and
# backups, follows TZ and logs no secret. Runs on the dev machine (arm64
# under QEMU) and on the server itself.
#
#   scripts/deploy-smoke.sh [-image ref] [-platform p] [-build] [-upgrade-db backup.db] [-keep]
#
#   -image ref         image to test (default: plantry-smoke-img:local, or
#                      ghcr.io/brokdar/plantry:latest without -build)
#   -platform p        linux/amd64, linux/arm64 or linux/arm/v7 (default: the
#                      host's; a foreign one runs under QEMU/binfmt)
#   -build             build the image first for the platform
#   -upgrade-db file   a `plantry backup` file; a secret.key beside it is used
#                      too. Adds the upgrade check.
#   -keep              keep the containers and volumes (names are printed)
#
# SMOKE_LIVE=1 (with PLANTRY_AI_PROVIDER, _MODEL and _API_KEY in the
# environment) adds one real help question: a few cents.
#
# Needs docker, curl and python3. Everything it creates is named
# plantry-smoke-<id>-*; it never touches other containers or volumes and
# never uses port 8080.
set -u

IMAGE="" SMOKE_PLATFORM="${SMOKE_PLATFORM:-}" BUILD=0 UPGRADE_DB="" KEEP=0
while [ $# -gt 0 ]; do
  case "$1" in
    -image) IMAGE="${2:?-image needs a value}"; shift 2 ;;
    -platform) SMOKE_PLATFORM="${2:?-platform needs a value}"; shift 2 ;;
    -build) BUILD=1; shift ;;
    -upgrade-db) UPGRADE_DB="${2:?-upgrade-db needs a value}"; shift 2 ;;
    -keep) KEEP=1; shift ;;
    -h|-help|--help) sed -n '2,/^set -u/p' "$0" | sed '$d;s/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $1 (try -h)" >&2; exit 2 ;;
  esac
done
for c in docker curl python3; do
  command -v "$c" >/dev/null || { echo "$c is required" >&2; exit 2; }
done
if [ -n "$UPGRADE_DB" ] && [ ! -f "$UPGRADE_DB" ]; then
  echo "-upgrade-db: no such file: $UPGRADE_DB" >&2; exit 2
fi

ID="$(date +%s)-$$"
P="plantry-smoke-$ID"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/$P.XXXXXX")"
HERE="$(cd "$(dirname "$0")/.." && pwd)"
PLATFORM_FLAG=()
if [ "$BUILD" = 1 ] && [ -z "$SMOKE_PLATFORM" ]; then
  case "$(uname -m)" in aarch64|arm64) SMOKE_PLATFORM=linux/arm64 ;; armv7l|armv6l) SMOKE_PLATFORM=linux/arm/v7 ;; *) SMOKE_PLATFORM=linux/amd64 ;; esac
fi
[ -n "${SMOKE_PLATFORM:-}" ] && PLATFORM_FLAG=(--platform "$SMOKE_PLATFORM")
if [ -z "$IMAGE" ]; then
  if [ "$BUILD" = 1 ]; then IMAGE="plantry-smoke-img:$(echo "$SMOKE_PLATFORM" | tr '/' '-')"; else IMAGE="ghcr.io/brokdar/plantry:latest"; fi
fi
EXPECT_VERSION=""
CONTAINERS=() VOLUMES=()

cleanup() {
  if [ "$KEEP" = 1 ]; then
    echo "kept: containers ${CONTAINERS[*]:-none}; volumes ${VOLUMES[*]:-none}; files $WORK"
    return
  fi
  for c in "${CONTAINERS[@]:-}"; do [ -n "$c" ] && docker rm -f "$c" >/dev/null 2>&1; done
  for v in "${VOLUMES[@]:-}"; do [ -n "$v" ] && docker volume rm "$v" >/dev/null 2>&1; done
  [ "$BUILD" = 1 ] && [[ "$IMAGE" == plantry-smoke-img:* ]] && docker rmi "$IMAGE" >/dev/null 2>&1
  rm -rf "$WORK"
}
trap cleanup EXIT

# ---------------------------------------------------------------- helpers

PORT=""
free_port() {
  python3 -I -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])'
}

# new_volume NAME: creates a volume and sets VOL to its name (not via $(...):
# the cleanup list must grow in this shell).
new_volume() {
  VOL="$P-$1"
  docker volume create "$VOL" >/dev/null && VOLUMES+=("$VOL")
}

# put_in_volume VOLUME SRC_FILE DEST_NAME: copies a file to the volume, owned
# by the image's plantry user.
put_in_volume() {
  docker run --rm "${PLATFORM_FLAG[@]}" --user 0 --name "$P-cp-$RANDOM" -v "$1:/data" -v "$(dirname "$2"):/src:ro" \
    --entrypoint sh "$IMAGE" -c "cp /src/$(basename "$2") /data/$3 && chown plantry:plantry /data/$3 && chmod 600 /data/$3" >/dev/null
}

# start NAME VOLUME [-e K=V ...]: runs a container on a free port; sets PORT.
start() {
  local name="$P-$1" vol="$2"; shift 2
  # A free port can be taken between picking and binding (Docker Desktop's
  # forwarder answers 500): try three.
  local try ok=0
  for try in 1 2 3; do
    PORT="$(free_port)"
    if docker run -d "${PLATFORM_FLAG[@]}" --name "$name" -p "127.0.0.1:$PORT:8080" -v "$vol:/data" "$@" "$IMAGE" >/dev/null; then ok=1; break; fi
    docker rm -f "$name" >/dev/null 2>&1
  done
  [ $ok = 1 ] || return 1
  CONTAINERS+=("$name")
  wait_health "$name"
}

wait_health() { # container: up to 180 s (QEMU starts slowly)
  local i
  for i in $(seq 1 180); do
    if curl -fsS "http://127.0.0.1:$PORT/api/health" >/dev/null 2>&1; then return 0; fi
    if [ "$(docker inspect -f '{{.State.Running}}' "$1" 2>/dev/null)" != "true" ]; then
      echo "container $1 stopped:"; docker logs --tail 20 "$1" 2>&1; return 1
    fi
    sleep 1
  done
  echo "no health after 180 s"; docker logs --tail 20 "$1" 2>&1; return 1
}

api() { # METHOD PATH [JSON]
  curl -sS -o "$WORK/body" -w '%{http_code}' -X "$1" -H 'Content-Type: application/json' \
    ${3:+-d "$3"} "http://127.0.0.1:$PORT$2"
}

jq_() { # python expression over the parsed JSON file $1 as j
  python3 -I -c 'import json,sys; j=json.load(open(sys.argv[1])); print(eval(sys.argv[2]))' "$1" "$2"
}

# counts CONTAINER: backs the running database up with the release binary and
# prints "foods plates plate_feedback chef.enabled goose_version" from the copy.
counts() {
  local c="$1" f="snap-$RANDOM.db"
  docker exec "$c" sh -c "mkdir -p /data/backup && plantry backup -out /data/backup/$f" >/dev/null 2>&1 || return 1
  mkdir -p "$WORK/snap-$f" && docker cp "$c:/data/backup/$f" "$WORK/snap-$f/$f" >/dev/null || return 1
  db_counts "$WORK/snap-$f/$f"
}

db_counts() { # db file
  python3 -I - "$1" <<'PY'
import sqlite3, sys
c = sqlite3.connect("file:%s?mode=ro" % sys.argv[1], uri=True)
n = lambda t: c.execute("SELECT COUNT(*) FROM %s" % t).fetchone()[0]
chef = c.execute("SELECT value FROM app_settings WHERE key='chef.enabled'").fetchone()
ver = c.execute("SELECT MAX(version_id) FROM goose_db_version").fetchone()[0]
print(n("foods"), n("plates"), n("plate_feedback"), chef[0] if chef else "unset", ver)
PY
}

# ------------------------------------------------------------ check runner

FAILED=0
declare -a SUMMARY
check() { # name function [args]
  local name="$1"; shift
  local t0 t1 out rc
  t0=$SECONDS
  # In this shell (not a subshell): checks hand containers and ports on.
  "$@" >"$WORK/out" 2>&1; rc=$?
  out="$(cat "$WORK/out")"
  t1=$SECONDS
  if [ $rc -eq 0 ]; then
    printf 'PASS  %-34s %4ss\n' "$name" $((t1 - t0))
    [ -n "$out" ] && printf '%s\n' "$out" | sed 's/^/        /' | tail -6
    SUMMARY+=("PASS $name $((t1 - t0))s")
  elif [ $rc -eq 77 ]; then
    printf 'SKIP  %-34s %4ss  %s\n' "$name" $((t1 - t0)) "$out"
    SUMMARY+=("SKIP $name")
  else
    printf 'FAIL  %-34s %4ss\n' "$name" $((t1 - t0))
    printf '%s\n' "$out" | sed 's/^/        /' | tail -25
    SUMMARY+=("FAIL $name $((t1 - t0))s")
    FAILED=1
  fi
}

# --------------------------------------------------------------- fixtures

SECRET_KEY="smoke$(python3 -I -c 'import secrets; print(secrets.token_hex(32))')"
API_KEY="sk-smoke-$(python3 -I -c 'import secrets; print(secrets.token_hex(12))')"

write_fake_script() {
  python3 -I - "$WORK/fake.json" <<'PY'
import json, sys
def turn(tool, inp, tokens=(500, 50)):
    return {"for_tool": tool, "stop_reason": "tool_use",
            "message": {"content": [{"type": "tool_use", "tool_use_id": "toolu_" + tool,
                                     "tool_name": tool, "tool_input": inp}]},
            "usage": {"input_tokens": tokens[0], "output_tokens": tokens[1]}}
script = {"capabilities": {"web_search": False}, "turns": [
    turn("chef_help_answer", {"lines": ["Chop the onions small.", "Even pieces cook evenly."]}),
    turn("report_import_check", {"prep_minutes": 10, "cook_minutes": 20,
         "time_reason": "The steps take about half an hour.", "step_edits": [], "flags": []}),
    turn("chef_find_search", {"urls": []}),
]}
json.dump(script, open(sys.argv[1], "w"))
PY
}

# ----------------------------------------------------------------- checks

VOL_A=""
ENV_A=()

check_fresh() {
  new_volume a; VOL_A="$VOL"
  write_fake_script
  put_in_volume "$VOL_A" "$WORK/fake.json" fake.json || return 1
  ENV_A=(-e PLANTRY_AI_PROVIDER=fake -e PLANTRY_AI_MODEL=fake-smoke -e PLANTRY_AI_FAKE_SCRIPT=/data/fake.json
         -e "PLANTRY_AI_API_KEY=$API_KEY" -e "PLANTRY_SECRET_KEY=$SECRET_KEY" -e TZ=Europe/Berlin)
  start a "$VOL_A" "${ENV_A[@]}" || return 1
  [ "$(api GET /api/health)" = 200 ] || { echo "health not 200"; return 1; }
  [ "$(jq_ "$WORK/body" 'j["status"]')" = ok ] || { echo "status not ok"; return 1; }
  local v; v="$(jq_ "$WORK/body" 'j["version"]')"
  if [ -n "$EXPECT_VERSION" ] && [ "$v" != "$EXPECT_VERSION" ]; then
    echo "version $v, image says $EXPECT_VERSION"; return 1
  fi
  [ -n "$v" ] && [ "$v" != dev ] || { echo "version is '$v'"; return 1; }
  echo "version $v"
  [ "$(api GET /api/settings)" = 200 ] &&
    [ "$(jq_ "$WORK/body" '[i.get("value") for i in j["items"] if i["key"] == "chef.enabled"]')" = "['true']" ] ||
    { echo "chef.enabled is not true on a fresh volume"; return 1; }
  [ "$(curl -s -o "$WORK/spa" -w '%{http_code}' "http://127.0.0.1:$PORT/")" = 200 ] && grep -qi '<div id="root"' "$WORK/spa" ||
    { echo "SPA not served"; return 1; }
  [ "$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/some/client/route")" = 200 ] || { echo "SPA fallback missing"; return 1; }
  logs_are_json "$P-a"
}

logs_are_json() {
  docker logs "$1" 2>&1 | python3 -I -c '
import json, sys
n = 0
for l in sys.stdin:
    l = l.strip()
    if not l: continue
    n += 1
    try: json.loads(l)
    except Exception: print("not JSON:", l[:200]); sys.exit(1)
if n == 0: print("no log lines"); sys.exit(1)
print(n, "JSON lines")'
}

check_startup_line() {
  docker logs "$P-a" 2>&1 | python3 -I -c '
import json, sys
for l in sys.stdin:
    try: j = json.loads(l)
    except Exception: continue
    if j.get("msg") == "server.start":
        need = ["version","schema_version","chef_enabled","ai_provider","ai_model","tz","local_time"]
        miss = [k for k in need if k not in j]
        if miss: print("missing", miss); sys.exit(1)
        if j["chef_enabled"] is not True or j["ai_provider"] != "fake": print("wrong values", j); sys.exit(1)
        print(l.strip()); sys.exit(0)
print("no server.start line"); sys.exit(1)'
}

check_ai_paths() {
  # A recipe, a slot and a plate tomorrow give the help question a cook session.
  local tag today tomorrow slot onion chili plate sid
  [ "$(api PUT /api/settings/ai.price_overrides '{"value":"{\"fake-smoke\":{\"in\":3,\"out\":15}}"}')" = 204 ] || { echo "price override: $(cat "$WORK/body")"; return 1; }
  api GET /api/today >/dev/null; today="$(jq_ "$WORK/body" 'j["date"]')"
  tomorrow="$(python3 -I -c 'import datetime,sys; print(datetime.date.fromisoformat(sys.argv[1])+datetime.timedelta(days=1))' "$today")"
  [ "$(api POST /api/settings/slots '{"name_key":"slot.smoke","icon":"moon","sort_order":999999,"active":true}')" = 201 ] || { echo "slot: $(cat "$WORK/body")"; return 1; }
  slot="$(jq_ "$WORK/body" 'j["id"]')"
  [ "$(api POST /api/foods '{"kind":"leaf","source":"manual","name":"Smoke onion","kcal_100g":40,"protein_100g":1.1,"fat_100g":0.1,"carbs_100g":9.3}')" = 201 ] || { echo "leaf: $(cat "$WORK/body")"; return 1; }
  onion="$(jq_ "$WORK/body" 'j["id"]')"
  [ "$(api POST /api/foods "{\"kind\":\"composed\",\"name\":\"Smoke chili\",\"role\":\"main\",\"reference_portions\":4,\"children\":[{\"child_id\":$onion,\"amount\":240,\"unit\":\"g\",\"grams\":240,\"sort_order\":0}],\"instructions\":[{\"step_number\":1,\"text\":\"Chop the onions.\"}]}")" = 201 ] || { echo "composed: $(cat "$WORK/body")"; return 1; }
  chili="$(jq_ "$WORK/body" 'j["id"]')"
  [ "$(api POST /api/plates "{\"date\":\"$tomorrow\",\"slot_id\":$slot,\"components\":[{\"food_id\":$chili,\"portions\":1}]}")" = 201 ] || { echo "plate: $(cat "$WORK/body")"; return 1; }
  sid="$(jq_ "$WORK/body" 'j["components"][0]["cook_session_id"]')"

  local code
  code="$(api POST /api/help "{\"session_id\":$sid,\"step_index\":0,\"kind\":\"explain_step\"}")"
  [ "$code" = 200 ] || { echo "help answered $code: $(cat "$WORK/body")"; return 1; }
  [ "$(jq_ "$WORK/body" 'len(j["answer"])')" -ge 1 ] || { echo "empty help answer"; return 1; }
  echo "help: $(jq_ "$WORK/body" 'j["answer"]')"

  code="$(api POST /api/import/extract "$(python3 -I -c '
import json
html = """<!DOCTYPE html><html><head><script type="application/ld+json">{"@context":"https://schema.org/","@type":"Recipe","name":"Smoke pan","recipeYield":"2","recipeIngredient":["400 g chicken","1 onion"],"recipeInstructions":[{"@type":"HowToStep","text":"Fry the chicken."},{"@type":"HowToStep","text":"Serve."}]}</script></head><body></body></html>"""
print(json.dumps({"html": html, "lang": "en"}))')")"
  [ "$code" = 200 ] || { echo "import answered $code: $(cat "$WORK/body")"; return 1; }
  [ "$(jq_ "$WORK/body" 'j["check"] is not None')" = True ] || { echo "import returned no check: $(head -c 400 "$WORK/body")"; return 1; }
  echo "import: checked, draft '$(jq_ "$WORK/body" 'j["draft"]["name"]')'"

  code="$(api POST /api/finds '{"dish":"smoke curry"}')"
  [ "$code" = 202 ] || { echo "find answered $code: $(cat "$WORK/body")"; return 1; }
  local fid i st=""
  fid="$(jq_ "$WORK/body" 'j["id"]')"
  for i in $(seq 1 90); do
    api GET "/api/finds/$fid" >/dev/null
    st="$(jq_ "$WORK/body" 'j["state"]')" || { echo "no state in $(head -c 300 "$WORK/body")"; return 1; }
    [ "$st" = searching ] || break
    sleep 1
  done
  # The script's search finds no page, so "failed" (nothing found) is the
  # expected end; what matters is that it ends and nothing answers 5xx.
  echo "find ended as '$st' after ${i}s"
  [ "$st" != searching ] || { echo "find still searching after 90 s"; return 1; }
  # No request so far answered 5xx.
  if docker logs "$P-a" 2>&1 | grep -E '"msg":"http.request".*"status":5[0-9][0-9]' >/dev/null; then
    docker logs "$P-a" 2>&1 | grep -E '"msg":"http.request".*"status":5[0-9][0-9]' | head -3; return 1
  fi
}

check_live() {
  [ "${SMOKE_LIVE:-}" = 1 ] || { echo "set SMOKE_LIVE=1 with the AI settings in the environment"; return 77; }
  [ -n "${PLANTRY_AI_PROVIDER:-}" ] && [ -n "${PLANTRY_AI_API_KEY:-}" ] || { echo "PLANTRY_AI_PROVIDER/_API_KEY not set"; return 77; }
  local vol sid code
  new_volume live; vol="$VOL"
  start live "$vol" -e PLANTRY_AI_PROVIDER -e PLANTRY_AI_MODEL -e PLANTRY_AI_API_KEY -e TZ=Europe/Berlin || return 1
  api GET /api/today >/dev/null
  local today tomorrow
  today="$(jq_ "$WORK/body" 'j["date"]')"
  tomorrow="$(python3 -I -c 'import datetime,sys; print(datetime.date.fromisoformat(sys.argv[1])+datetime.timedelta(days=1))' "$today")"
  api POST /api/settings/slots '{"name_key":"slot.smoke","icon":"moon","sort_order":999999,"active":true}' >/dev/null
  local slot; slot="$(jq_ "$WORK/body" 'j["id"]')"
  api POST /api/foods '{"kind":"composed","name":"Smoke soup","role":"main","reference_portions":2,"instructions":[{"step_number":1,"text":"Heat the stock and add the vegetables."}]}' >/dev/null
  local food; food="$(jq_ "$WORK/body" 'j["id"]')"
  api POST /api/plates "{\"date\":\"$tomorrow\",\"slot_id\":$slot,\"components\":[{\"food_id\":$food,\"portions\":1}]}" >/dev/null
  sid="$(jq_ "$WORK/body" 'j["components"][0]["cook_session_id"]')"
  code="$(api POST /api/help "{\"session_id\":$sid,\"step_index\":0,\"kind\":\"explain_step\"}")"
  [ "$code" = 200 ] || { echo "live help answered $code: $(cat "$WORK/body")"; return 1; }
  echo "answer: $(jq_ "$WORK/body" 'j["answer"]')"
}

check_restart() {
  local before after
  before="$(counts "$P-a")" || { echo "backup failed"; return 1; }
  docker restart "$P-a" >/dev/null && wait_health "$P-a" || return 1
  after="$(counts "$P-a")" || return 1
  [ "$before" = "$after" ] || { echo "restart changed data: [$before] -> [$after]"; return 1; }
  # Recreate: a new container on the same volume.
  save_logs "$P-a"
  docker rm -f "$P-a" >/dev/null
  start a2 "$VOL_A" "${ENV_A[@]}" || return 1
  after="$(counts "$P-a2")" || return 1
  [ "$before" = "$after" ] || { echo "recreate changed data: [$before] -> [$after]"; return 1; }
  [ "$(api GET /api/foods)" = 200 ] && grep -q 'Smoke chili' "$WORK/body" || { echo "recipe missing after recreate"; return 1; }
  echo "counts [$before]"
}

check_backup_restore() {
  local before dir="$WORK/restore" vol after
  before="$(counts "$P-a2")" || return 1
  mkdir -p "$dir"
  docker exec "$P-a2" sh -c 'plantry backup -out /data/backup/restore-test.db' >/dev/null 2>&1 || { echo "backup command failed"; return 1; }
  docker cp "$P-a2:/data/backup/restore-test.db" "$dir/plantry.db" >/dev/null || return 1
  docker cp "$P-a2:/data/backup/secret.key" "$dir/secret.key" >/dev/null 2>&1 || true
  new_volume restored; vol="$VOL"
  put_in_volume "$vol" "$dir/plantry.db" plantry.db || return 1
  # The key comes from PLANTRY_SECRET_KEY here, so no secret.key file exists
  # (backup warns and skips it); restore with the same env.
  [ -f "$dir/secret.key" ] && { put_in_volume "$vol" "$dir/secret.key" secret.key || return 1; }
  start restored "$vol" -e "PLANTRY_SECRET_KEY=$SECRET_KEY" -e TZ=Europe/Berlin || return 1
  after="$(counts "$P-restored")" || return 1
  [ "$before" = "$after" ] || { echo "restore differs: [$before] vs [$after]"; return 1; }
  echo "counts [$after]"
  save_logs "$P-restored"
  docker rm -f "$P-restored" >/dev/null
}

check_upgrade() {
  [ -n "$UPGRADE_DB" ] || { echo "no -upgrade-db given"; return 77; }
  local dir="$WORK/upgrade" vol want got
  mkdir -p "$dir"
  cp "$UPGRADE_DB" "$dir/plantry.db"
  [ -f "$(dirname "$UPGRADE_DB")/secret.key" ] && cp "$(dirname "$UPGRADE_DB")/secret.key" "$dir/secret.key"
  want="$(db_counts "$dir/plantry.db")" || { echo "cannot read $UPGRADE_DB"; return 1; }
  new_volume upgrade; vol="$VOL"
  put_in_volume "$vol" "$dir/plantry.db" plantry.db || return 1
  [ -f "$dir/secret.key" ] && { put_in_volume "$vol" "$dir/secret.key" secret.key || return 1; }
  start upgrade "$vol" -e TZ=Europe/Berlin || return 1
  got="$(counts "$P-upgrade")" || return 1
  # foods plates plate_feedback chef.enabled goose_version
  set -- $want; local wf=$1 wp=$2 wb=$3 wc=$4 wv=$5
  set -- $got;  local gf=$1 gp=$2 gb=$3 gc=$4 gv=$5
  echo "backup [$want] -> running [$got]"
  [ "$wf $wp $wb" = "$gf $gp $gb" ] || { echo "row counts changed"; return 1; }
  # A backup that never stored the switch gets the default; a stored one stays.
  if [ "$wc" != unset ] && [ "$wc" != "$gc" ]; then echo "chef.enabled changed: $wc -> $gc"; return 1; fi
  [ "$gv" -ge "$wv" ] || { echo "schema went backwards"; return 1; }
  if docker logs "$P-upgrade" 2>&1 | grep -q '"level":"ERROR"'; then docker logs "$P-upgrade" 2>&1 | grep '"level":"ERROR"' | head -3; return 1; fi
}

check_tz() {
  local vol want_today want_off line
  new_volume tz; vol="$VOL"
  start tz "$vol" -e TZ=Europe/Berlin || return 1
  want_today="$(TZ=Europe/Berlin date +%F)"
  want_off="$(TZ=Europe/Berlin date +%:z)"
  api GET /api/today >/dev/null
  [ "$(jq_ "$WORK/body" 'j["date"]')" = "$want_today" ] || { echo "today $(jq_ "$WORK/body" 'j["date"]'), Berlin says $want_today"; return 1; }
  line="$(docker logs "$P-tz" 2>&1 | grep '"msg":"server.start"')"
  case "$line" in *"\"local_time\":\""*"$want_off\""*) ;; *) echo "server.start local_time lacks offset $want_off: $line"; return 1 ;; esac
  case "$line" in *'"tz":"CET"'*|*'"tz":"CEST"'*) ;; *) echo "server.start tz is not CET/CEST: $line"; return 1 ;; esac
  echo "today $want_today, offset $want_off"
}

# save_logs CONTAINER: keeps its log in $WORK before the container goes.
save_logs() { docker logs "$1" >"$WORK/logs-${1##*-}.txt" 2>&1; }

# The keys set above (container a, a2, restored) and any generated
# secret.key (upgrade) must not appear in any container's log.
check_no_secrets() {
  local c f k bad=0 n=0
  for c in "${CONTAINERS[@]}"; do
    docker inspect "$c" >/dev/null 2>&1 && save_logs "$c"
  done
  for f in "$WORK"/logs-*.txt; do
    n=$((n + 1))
    if grep -qF -e "$API_KEY" -e "$SECRET_KEY" "$f"; then echo "a key appears in $(basename "$f")"; bad=1; fi
  done
  for c in "$P-upgrade" "$P-tz"; do
    docker inspect "$c" >/dev/null 2>&1 || continue
    k="$(docker exec "$c" sh -c 'cat /data/secret.key 2>/dev/null' || true)"
    if [ -n "$k" ] && grep -qF "$k" "$WORK/logs-${c##*-}.txt"; then echo "secret.key content in logs of $c"; bad=1; fi
  done
  echo "scanned $n logs"
  return $bad
}

# ------------------------------------------------------------------- main

echo "image: $IMAGE ${SMOKE_PLATFORM:+(platform $SMOKE_PLATFORM)}  id: $ID"
if [ "$BUILD" = 1 ]; then
  EXPECT_VERSION="0.0.0-smoke"
  t0=$SECONDS
  if docker buildx build --platform "$SMOKE_PLATFORM" --load --build-arg "VERSION=$EXPECT_VERSION" \
      -t "$IMAGE" "$HERE" >"$WORK/build.log" 2>&1; then
    printf 'PASS  %-34s %4ss\n' "build $SMOKE_PLATFORM" $((SECONDS - t0))
  else
    printf 'FAIL  %-34s %4ss\n' "build $SMOKE_PLATFORM" $((SECONDS - t0)); tail -30 "$WORK/build.log"; exit 1
  fi
else
  docker image inspect "$IMAGE" >/dev/null 2>&1 || docker pull "${PLATFORM_FLAG[@]}" "$IMAGE" >/dev/null || { echo "cannot pull $IMAGE"; exit 1; }
  lv="$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.version"}}' "$IMAGE" 2>/dev/null)"
  [ -n "$lv" ] && [ "$lv" != "<no value>" ] && EXPECT_VERSION="$lv"
fi
docker image inspect -f 'arch {{.Architecture}}{{.Variant}}/{{.Os}}' "$IMAGE" 2>/dev/null

check "fresh volume: health, SPA, JSON logs" check_fresh
check "startup line"                     check_startup_line
check "fake AI: help, import, find"      check_ai_paths
check "live provider: one help question" check_live
check "restart and recreate keep data"   check_restart
check "backup and restore round trip"    check_backup_restore
check "upgrade from -upgrade-db backup"  check_upgrade
check "TZ=Europe/Berlin is followed"     check_tz
check "no secret in docker logs"         check_no_secrets

echo
printf '%s\n' "${SUMMARY[@]}"
SKIPPED=()
for line in "${SUMMARY[@]}"; do
  case "$line" in "SKIP "*) SKIPPED+=("${line#SKIP }") ;; esac
done
if [ $FAILED -ne 0 ]; then
  echo "SMOKE FAILED"
elif [ ${#SKIPPED[@]} -gt 0 ]; then
  skipped_list="$(printf '%s, ' "${SKIPPED[@]}")"
  echo "SMOKE OK (${#SKIPPED[@]} skipped: ${skipped_list%, })"
else
  echo "SMOKE OK"
fi
exit $FAILED
