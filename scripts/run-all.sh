#!/bin/bash
# Run all validation scripts in order
# Steps 4 and 5 are skipped automatically if OPENROUTER_API_KEY is not set

SCRIPT_DIR="$(dirname "$0")"
FAILED=0

for script in "$SCRIPT_DIR"/0[1-7]-*.sh; do
  echo ""
  echo "========================================"
  echo "Running: $(basename "$script")"
  echo "========================================"
  if bash "$script"; then
    echo ">>> $(basename "$script"): OK"
  else
    echo ">>> $(basename "$script"): FAILED"
    FAILED=1
  fi
  echo ""
done

if [ $FAILED -eq 0 ]; then
  echo "========================================"
  echo "ALL VALIDATION STEPS PASSED"
  echo "========================================"
else
  echo "========================================"
  echo "SOME VALIDATION STEPS FAILED"
  echo "========================================"
  exit 1
fi
