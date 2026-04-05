#!/bin/bash
echo '=== Starting short self-test ==='
sudo /usr/sbin/smartctl -t short /dev/sda 2>&1
echo ''
echo 'Test started. Check results in ~2 min with:'
echo 'bash /home/alex/picoclaw/workspace/scripts/disk_status.sh'
