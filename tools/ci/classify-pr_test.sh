#!/usr/bin/env bash
# classify-pr_test.sh — table tests for tools/ci/classify-pr.sh.
#
# Each case builds a branch off a hermetic temp repo and runs the classifier
# with the case's env. No network, no user git config.

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLASSIFY="$HERE/classify-pr.sh"

fails=0
n=0
assert_eq() {
  if [ "$2" = "$3" ]; then echo "ok   - $1"
  else echo "FAIL - $1"; echo "        want: $2"; echo "        got:  $3"; fails=$((fails + 1)); fi
}
assert_contains() {
  case "$3" in
    *"$2"*) echo "ok   - $1" ;;
    *) echo "FAIL - $1"; echo "        expected to contain: $2"; fails=$((fails + 1)) ;;
  esac
}

[ -x "$CLASSIFY" ] || { echo "FAIL - $CLASSIFY missing or not executable"; exit 1; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
REPO_DIR="$TMP/repo"
mkdir -p "$REPO_DIR"
cd "$REPO_DIR" || exit 2

export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
git init -q -b main
git config user.name test
git config user.email test@example.invalid
git config commit.gpgsign false

mkdir -p deploy/kubernetes
cat > deploy/kubernetes/deployment.yaml <<'EOF'
spec:
  template:
    spec:
      containers:
        - name: harbormaster
          image: registry.tumidanski/jtumidanski/harbormaster:sha-780b019
          imagePullPolicy: IfNotPresent
-- evil
EOF
echo "readme" > README.md
git add -A
git commit -q -m base
git update-ref refs/remotes/origin/main HEAD

DEP=deploy/kubernetes/deployment.yaml
OLD_TAG=sha-780b019

pin_tag() { sed -i "s/harbormaster:$OLD_TAG/harbormaster:$1/" "$DEP"; }
pin() { pin_tag sha-abc1234; }
policy_always() { sed -i 's/IfNotPresent/Always/' "$DEP"; }

# run_case <name> <want> <branch> <env-overrides...> -- <edit function and args>
# Env defaults: HEAD_REF=<branch> HEAD_REPO=REPO=jtumidanski/Harbormaster BASE_REF=main
run_case() {
  local name="$1" want="$2" branch="$3"; shift 3
  local head_ref="$branch" head_repo=jtumidanski/Harbormaster base_ref=main
  while [ "$1" != "--" ]; do
    case "$1" in
      HEAD_REF=*) head_ref="${1#HEAD_REF=}" ;;
      HEAD_REPO=*) head_repo="${1#HEAD_REPO=}" ;;
      BASE_REF=*) base_ref="${1#BASE_REF=}" ;;
    esac
    shift
  done
  shift
  n=$((n + 1))
  git switch -q -c "case-$n" origin/main
  "$@"
  git add -A
  git commit -q -m "case $name"
  local out err rc
  err="$TMP/stderr"
  out="$(HEAD_REF="$head_ref" HEAD_REPO="$head_repo" REPO=jtumidanski/Harbormaster BASE_REF="$base_ref" \
    "$CLASSIFY" 2>"$err")"
  rc=$?
  assert_eq "$name: stdout" "bump-only=$want" "$out"
  assert_eq "$name: one stdout line" "1" "$(printf '%s\n' "$out" | wc -l | tr -d ' ')"
  assert_eq "$name: exit 0" "0" "$rc"
  if [ "$want" = false ]; then
    assert_contains "$name: stderr explains" "classified full:" "$(cat "$err")"
  fi
  git switch -q main
}

bot=bot/image-bump-abc1234

pin_and_readme() { pin; echo more >> README.md; }
pin_and_rm() { pin; git rm -q README.md; }
pin_and_policy() { pin; policy_always; }
swap_policy_for_image() {
  sed -i '/imagePullPolicy/d' "$DEP"
  echo "          image: registry.tumidanski/jtumidanski/harbormaster:sha-abc1234" >> "$DEP"
}

pin_and_deleted_dashes() { pin; sed -i '/^-- evil$/d' "$DEP"; }
pin_and_added_pluses() { pin; printf '%s\n' '++ x' >> "$DEP"; }

run_case "bot pin" true "$bot" -- pin
run_case "bot pin + deleted '-- x' line" false "$bot" -- pin_and_deleted_dashes
run_case "bot pin + added '++ x' line" false "$bot" -- pin_and_added_pluses
run_case "human pin" false task-007-x -- pin
run_case "bot + second file" false "$bot" -- pin_and_readme
run_case "bot deletes a file" false "$bot" -- pin_and_rm
run_case "bot non-pin edit" false "$bot" -- policy_always
run_case "bot pin + extra line" false "$bot" -- pin_and_policy
run_case "removed line not an image" false "$bot" -- swap_policy_for_image
run_case "short tag" false "$bot" -- pin_tag sha-abc12
run_case "uppercase tag" false "$bot" -- pin_tag sha-ABC1234
run_case "fork" false "$bot" HEAD_REPO=mallory/Harbormaster -- pin
run_case "lookalike prefix" false xbot/image-bump-abc1234 -- pin
run_case "empty HEAD_REF" false "$bot" HEAD_REF= -- pin
run_case "empty HEAD_REPO" false "$bot" HEAD_REPO= -- pin
run_case "empty BASE_REF" false "$bot" BASE_REF= -- pin
run_case "missing base" false "$bot" BASE_REF=nope -- pin

if [ "$fails" -ne 0 ]; then
  echo "FAIL: $fails assertion(s) failed"
  exit 1
fi
echo "PASS: all classify-pr cases"
