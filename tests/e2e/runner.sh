#!/usr/bin/env bash
set -e

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

printf "\n%b========================================================================%b\n" "$CYAN" "$NC"
printf "%b         AGYREMOTE E2E TEST SUITE RUNNER (4-TIER MATRIX)                 %b\n" "$BOLD" "$NC"
printf "%b========================================================================%b\n\n" "$CYAN" "$NC"

TIERS=(
  "Tier 1: Feature Coverage|tier1_features.test.mjs"
  "Tier 2: Boundary Value Analysis|tier2_boundaries.test.mjs"
  "Tier 3: Pairwise Interactions|tier3_interactions.test.mjs"
  "Tier 4: Real-World Scenarios|tier4_realworld.test.mjs"
)

TOTAL_TESTS=0
TOTAL_PASS=0
TOTAL_SKIP=0
TOTAL_FAIL=0
TOTAL_DURATION=0
SUITE_FAILED=0

RESULTS=()

for ENTRY in "${TIERS[@]}"; do
  NAME="${ENTRY%%|*}"
  FILE="${ENTRY##*|}"

  printf "%b[RUNNING]%b %s (%s)...\n" "$YELLOW" "$NC" "$NAME" "$FILE"

  OUTPUT=$(node --test "$DIR/$FILE" 2>&1) || STATUS=$?
  STATUS=${STATUS:-0}

  T_COUNT=$(echo "$OUTPUT" | grep -E "ℹ tests [0-9]+" | awk '{print $3}' | tail -n 1)
  P_COUNT=$(echo "$OUTPUT" | grep -E "ℹ pass [0-9]+" | awk '{print $3}' | tail -n 1)
  S_COUNT=$(echo "$OUTPUT" | grep -E "ℹ skipped [0-9]+" | awk '{print $3}' | tail -n 1)
  F_COUNT=$(echo "$OUTPUT" | grep -E "ℹ fail [0-9]+" | awk '{print $3}' | tail -n 1)
  D_MS=$(echo "$OUTPUT" | grep -E "ℹ duration_ms [0-9.]+" | awk '{printf "%.0f", $3}' | tail -n 1)

  T_COUNT=${T_COUNT:-0}
  P_COUNT=${P_COUNT:-0}
  S_COUNT=${S_COUNT:-0}
  F_COUNT=${F_COUNT:-0}
  D_MS=${D_MS:-0}

  TOTAL_TESTS=$((TOTAL_TESTS + T_COUNT))
  TOTAL_PASS=$((TOTAL_PASS + P_COUNT))
  TOTAL_SKIP=$((TOTAL_SKIP + S_COUNT))
  TOTAL_FAIL=$((TOTAL_FAIL + F_COUNT))
  TOTAL_DURATION=$((TOTAL_DURATION + D_MS))

  if [ "$STATUS" -ne 0 ] || [ "$F_COUNT" -gt 0 ]; then
    SUITE_FAILED=1
    printf "%b[FAILED]%b  %s (%d failed)\n" "$RED" "$NC" "$NAME" "$F_COUNT"
    echo "$OUTPUT" | grep -E "(✖|AssertionError|Error:)" | head -n 10
  else
    printf "%b[PASSED]%b  %s (pass: %d, skipped: %d)\n\n" "$GREEN" "$NC" "$NAME" "$P_COUNT" "$S_COUNT"
  fi

  RESULTS+=("$NAME|$T_COUNT|$P_COUNT|$S_COUNT|$F_COUNT|${D_MS}ms")
  unset STATUS
done

printf "\n%b------------------------------------------------------------------------------------------------%b\n" "$CYAN" "$NC"
printf "%b%-34s %8s %8s %10s %8s %12s%b\n" "$BOLD" "Test Suite Tier" "Total" "Passed" "Skipped" "Failed" "Duration" "$NC"
printf "%b------------------------------------------------------------------------------------------------%b\n" "$CYAN" "$NC"

for RES in "${RESULTS[@]}"; do
  IFS='|' read -r R_NAME R_TOTAL R_PASS R_SKIP R_FAIL R_DUR <<< "$RES"
  printf "%-34s %8s %8s %10s %8s %12s\n" "$R_NAME" "$R_TOTAL" "$R_PASS" "$R_SKIP" "$R_FAIL" "$R_DUR"
done

printf "%b------------------------------------------------------------------------------------------------%b\n" "$CYAN" "$NC"
printf "%b%-34s %8d %8d %10d %8d %10dms%b\n" "$BOLD" "Total" "$TOTAL_TESTS" "$TOTAL_PASS" "$TOTAL_SKIP" "$TOTAL_FAIL" "$TOTAL_DURATION" "$NC"
printf "%b================================================================================================%b\n\n" "$CYAN" "$NC"

if [ "$SUITE_FAILED" -eq 0 ]; then
  printf "%b>>> ALL E2E TEST TIERS VERIFIED SUCCESSFULLY (EXIT CODE 0) <<<%b\n\n" "$GREEN" "$NC"
  exit 0
else
  printf "%b>>> E2E TEST SUITE ENCOUNTERED FAILURES (EXIT CODE 1) <<<%b\n\n" "$RED" "$NC"
  exit 1
fi
