#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
set -a
source <(tr -d '\r' < .env)
set +a

sql() {
  docker compose exec -T db sh -c 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot campusclaw -N -e "$1"' sh "$1"
}
counts() {
  sql 'SELECT (SELECT COUNT(*) FROM materials), (SELECT COUNT(*) FROM knowledge_entries)'
  docker compose exec -T api sh -c 'find /uploads -maxdepth 1 -type f | wc -l'
}

before="$(counts)"
sql 'DROP TRIGGER IF EXISTS campusclaw_verify_fail'
trap "sql 'DROP TRIGGER IF EXISTS campusclaw_verify_fail'" EXIT
sql "CREATE TRIGGER campusclaw_verify_fail BEFORE INSERT ON knowledge_entries FOR EACH ROW SET NEW.material_id = IF(NEW.content = 'TRIGGER_FAIL', 0, NEW.material_id)"

sample="$(mktemp --suffix=.txt)"
trap 'rm -f "$sample"; sql "DROP TRIGGER IF EXISTS campusclaw_verify_fail"' EXIT
printf 'TRIGGER_FAIL' > "$sample"
token="$(printf '{"username":"teacher_a","password":"%s"}' "$TEACHER_A_PASSWORD" | curl -fsS -H 'Content-Type: application/json' --data-binary @- "http://localhost:${WEB_PORT:-8080}/api/login" | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')"
status="$(curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $token" -F "file=@$sample;filename=rollback-test.txt" "http://localhost:${WEB_PORT:-8080}/api/materials")"
after="$(counts)"
test "$status" = 503
test "$before" = "$after"
printf 'Second insert failed: HTTP %s; row and file counts unchanged.\n' "$status"
printf 'Before and after: %s\n' "$(printf '%s' "$before" | tr '\n' ' ')"
