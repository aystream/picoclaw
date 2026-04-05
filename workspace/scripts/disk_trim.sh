#!/bin/bash
echo '=== Running fstrim on all mounted filesystems ==='
sudo /usr/sbin/fstrim -av 2>&1
echo ''
echo '=== SMART status after TRIM ==='
sudo /usr/sbin/smartctl -a /dev/sda 2>&1 | grep -E 'Reallocated|Current_Pending|Offline_Uncorrectable|Available_Reservd|UDMA_CRC|health'
