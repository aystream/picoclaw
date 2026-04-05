#!/bin/bash
echo '=== SMART Health ==='
sudo /usr/sbin/smartctl -H /dev/sda 2>&1
echo ''
echo '=== Key Counters ==='
sudo /usr/sbin/smartctl -A /dev/sda 2>&1 | grep -E 'Reallocated|Current_Pending|Offline_Uncorrectable|Available_Reservd|UDMA_CRC|Power_On_Hours|Wear_Leveling'
echo ''
echo '=== Self-test Log ==='
sudo /usr/sbin/smartctl -l selftest /dev/sda 2>&1 | tail -10
echo ''
echo '=== Kernel disk errors ==='
dmesg 2>/dev/null | grep -iE 'ata.*error|I/O error|sda' | tail -10 || echo 'no dmesg access'
