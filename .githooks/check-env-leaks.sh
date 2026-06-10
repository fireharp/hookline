#!/usr/bin/env sh
# Reject staged changes that contain literal values from a gitignored .env.

set -eu

env_file=".env"
[ -f "$env_file" ] || exit 0

min_len=6
exit_code=0
diff_output=$(git diff --cached --unified=0 -- ':(exclude).env' ':(exclude).env.*' ':(exclude).env.example' 2>/dev/null || true)
[ -n "$diff_output" ] || exit 0

redact_value() {
  value=$1
  len=${#value}
  if [ "$len" -le 8 ]; then
    printf '<redacted:%s chars>' "$len"
  else
    prefix=$(printf '%s' "$value" | cut -c 1-3)
    suffix=$(printf '%s' "$value" | rev | cut -c 1-3 | rev)
    printf '%s...%s (%s chars)' "$prefix" "$suffix" "$len"
  fi
}

while IFS= read -r line; do
  case "$line" in
    ''|'#'*) continue ;;
  esac

  key=$(printf '%s\n' "$line" | sed -E 's/^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)[[:space:]]*\??=.*/\1/')
  value=$(printf '%s\n' "$line" | sed -E 's/^[[:space:]]*[A-Za-z_][A-Za-z0-9_]*[[:space:]]*\??=[[:space:]]*//')
  [ "$key" != "$line" ] || continue

  value=$(printf '%s' "$value" | sed -E "s/^['\"]//; s/['\"]\$//")
  [ "${#value}" -ge "$min_len" ] || continue

  hits=$(printf '%s\n' "$diff_output" | grep -F -- "$value" | grep -E '^\+[^+]' || true)
  if [ -n "$hits" ]; then
    if [ "$exit_code" -eq 0 ]; then
      echo "Staged changes contain values from $env_file:" >&2
      echo "" >&2
    fi
    printf '  %s: %s\n' "$key" "$(redact_value "$value")" >&2
    echo "$hits" | sed -E 's/^([+]).*/\1<redacted staged line>/' | sed 's/^/    /' >&2
    echo "" >&2
    exit_code=1
  fi
done < "$env_file"

if [ "$exit_code" -ne 0 ]; then
  echo "These values come from your local $env_file and should not be committed." >&2
  echo "Move the literal into an environment variable, config value, or safe placeholder." >&2
fi

exit "$exit_code"
