#!/usr/bin/env bash
# Simulates a Face ID 5 with curl, so the demo runs without a real device.
# Usage: ./scripts/simulate-device.sh [URL] [SN]
set -euo pipefail
URL="${1:-http://localhost:8081}"
SN="${2:-DEMO-FACEID5-001}"

step() { printf '\n=== %s ===\n' "$1"; }

step "1. Device powers on and asks for settings (handshake)"
curl -s "$URL/iclock/cdata?SN=$SN&options=all&pushver=2.4.1&language=69"; echo

step "2. System queues INFO and add_user"
curl -s -X POST "$URL/command" -H 'Content-Type: application/json' -d "{\"sn\":\"$SN\",\"action\":\"info\"}" >/dev/null
curl -s -X POST "$URL/command" -H 'Content-Type: application/json' -d "{\"sn\":\"$SN\",\"action\":\"add_user\",\"pin\":\"1001\",\"name\":\"Ali Demo\"}"

step "3. Device polls getrequest (gets first command)"
CMD=$(curl -s "$URL/iclock/getrequest?SN=$SN"); echo "$CMD"
ID=$(echo "$CMD" | cut -d: -f2)

step "4. Device reports INFO result"
printf 'ID=%s&Return=0&CMD=INFO\nDeviceName=Face ID 5\nFWVersion=Ver 8.0.4.2\nUserCount=1\n' "$ID" |
  curl -s -X POST "$URL/iclock/devicecmd?SN=$SN" --data-binary @-; echo

step "5. Device takes second command and reports success"
CMD=$(curl -s "$URL/iclock/getrequest?SN=$SN"); echo "$CMD"
ID=$(echo "$CMD" | cut -d: -f2)
curl -s -X POST "$URL/iclock/devicecmd?SN=$SN" --data-binary "ID=$ID&Return=0&CMD=DATA"; echo

step "6. Employee scans face -> device pushes ATTLOG (realtime)"
NOW=$(date '+%Y-%m-%d %H:%M:%S')
printf '1001\t%s\t0\t15\t0\t0\n' "$NOW" |
  curl -s -X POST "$URL/iclock/cdata?SN=$SN&table=ATTLOG&Stamp=9999" --data-binary @-; echo

step "7. No more commands"
curl -s "$URL/iclock/getrequest?SN=$SN"; echo

step "8. System checks device status"
curl -s "$URL/status?sn=$SN"

step "9. Command history"
curl -s "$URL/command?sn=$SN"
