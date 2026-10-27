#!/bin/bash
set -euo pipefail

echo "=== Verification Check 1: Process Check ==="
if pgrep -x agyremote > /dev/null 2>&1; then
    echo "FAIL: agyremote process still running"
    exit 1
fi
if pgrep -f "/opt/data/home/AgyRemoteTele/daemon.sh" > /dev/null 2>&1; then
    echo "FAIL: daemon.sh process still running"
    exit 1
fi
echo "Zero legacy processes running: PASS"

echo "=== Verification Check 2: Lock Status ==="
flock -n /opt/data/home/AgyRemoteTele/data/agyremote.lock -c "echo Flock non-blocking acquire: PASS"

echo "=== Verification Check 3: Backup Archive Integrity ==="
tar -tzf /opt/data/home/agyremote/AgyRemoteTele_backup.tar.gz > /dev/null
echo "Backup tar integrity: PASS"

echo "=== Verification Check 4: Critical Files in Archive ==="
for file in "AgyRemoteTele/.env" "AgyRemoteTele/data/sessions.json" "AgyRemoteTele/bin/agyremote" "AgyRemoteTele/daemon.sh"; do
    tar -tzf /opt/data/home/agyremote/AgyRemoteTele_backup.tar.gz "$file" > /dev/null
    echo "Found $file in archive: PASS"
done

echo "=== Verification Check 5: Execute Permissions Revoked ==="
if [ -x /opt/data/home/AgyRemoteTele/daemon.sh ] || [ -x /opt/data/home/AgyRemoteTele/bin/agyremote ]; then
    echo "FAIL: Execute permissions still set on daemon.sh or bin/agyremote"
    exit 1
fi
echo "Permissions revoked: PASS"

echo "=== Verification Check 6: DECOMMISSIONED Marker ==="
if [ ! -s /opt/data/home/AgyRemoteTele/DECOMMISSIONED.md ]; then
    echo "FAIL: DECOMMISSIONED.md missing or empty"
    exit 1
fi
echo "DECOMMISSIONED marker file exists: PASS"

echo "ALL VERIFICATION CHECKS PASSED!"
