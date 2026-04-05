#!/bin/bash
# Weekly disk health monitor — compares with previous state
STATE_FILE=/home/alex/picoclaw/workspace/memory/disk_state.txt
PREV=""
[ -f "$STATE_FILE" ] && PREV=$(cat "$STATE_FILE")

CURRENT=$(sudo /usr/sbin/smartctl -A /dev/sda 2>&1 | grep -E 'Reallocated_Sector|Current_Pending|Offline_Uncorrectable|Available_Reservd|Wear_Leveling|Power_On_Hours')

echo "$CURRENT" > "$STATE_FILE"

if [ -z "$PREV" ]; then
    echo "First run. Current SMART counters:"
    echo "$CURRENT"
else
    echo "=== SMART Delta ==="
    diff <(echo "$PREV") <(echo "$CURRENT") || echo "(no changes)"
    echo ""
    echo "=== Current values ==="
    echo "$CURRENT"
fi
