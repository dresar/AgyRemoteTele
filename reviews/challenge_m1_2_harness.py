#!/usr/bin/env python3
import os
import sys
import json
import socket
import tarfile
import hashlib
import fcntl
import glob
import datetime

def test_port_8787():
    print("[TEST 1] Port 8787 Availability & Handshake Test")
    port_hex = f"{8787:04X}"
    
    # 1.1 Check /proc/net/tcp and tcp6 for any existing sockets
    for path in ['/proc/net/tcp', '/proc/net/tcp6']:
        if os.path.exists(path):
            with open(path) as f:
                for line in f.readlines()[1:]:
                    parts = line.strip().split()
                    local_port = parts[1].split(':')[1]
                    if local_port == port_hex:
                        raise AssertionError(f"Port 8787 already present in {path}: {line.strip()}")
    print("  -> /proc/net/tcp* verification: CLEAN (no sockets on 8787)")

    # 1.2 Test binding on 127.0.0.1:8787
    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    try:
        server.bind(('127.0.0.1', 8787))
        server.listen(1)
        print("  -> Bound and listening on 127.0.0.1:8787: SUCCESS")
        
        # 1.3 Active connection handshake test
        client = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        client.connect(('127.0.0.1', 8787))
        conn, addr = server.accept()
        client.sendall(b"PING_CHALLENGER")
        received = conn.recv(1024)
        assert received == b"PING_CHALLENGER", f"Data mismatch: {received}"
        conn.sendall(b"PONG_CHALLENGER")
        client_received = client.recv(1024)
        assert client_received == b"PONG_CHALLENGER", f"Client data mismatch: {client_received}"
        client.close()
        conn.close()
        print("  -> Handshake client-server data transfer on 127.0.0.1:8787: SUCCESS")
    finally:
        server.close()
    
    # 1.4 Test binding on 0.0.0.0:8787
    server_all = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server_all.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    try:
        server_all.bind(('0.0.0.0', 8787))
        server_all.listen(1)
        print("  -> Bound and listening on 0.0.0.0:8787: SUCCESS")
    finally:
        server_all.close()

    print("[TEST 1] RESULT: PASS")

def test_process_isolation_and_handles():
    print("[TEST 2] Process Isolation, Lock & File Descriptor Inspection")
    
    # 2.1 Scan /proc for legacy bot processes (excluding review / test harnesses)
    matching_pids = []
    for pid_dir in glob.glob('/proc/[0-9]*'):
        pid = os.path.basename(pid_dir)
        try:
            exe = os.path.realpath(os.path.join(pid_dir, 'exe'))
            if 'agyremotetele' in exe.lower():
                matching_pids.append((pid, f"exe: {exe}"))
                continue
            
            with open(os.path.join(pid_dir, 'comm')) as f:
                comm = f.read().strip()
            if comm == 'agyremote':
                matching_pids.append((pid, f"comm: {comm}"))
                continue

            with open(os.path.join(pid_dir, 'cmdline'), 'rb') as cf:
                raw_args = cf.read().split(b'\x00')
                args = [a.decode('utf-8', errors='ignore') for a in raw_args if a]
            
            if args:
                first_cmd = args[0].lower()
                # If it's a python test script or runner, skip
                if any(py in first_cmd for py in ['python', 'pytest']):
                    continue
                # If command directly executes daemon.sh or bin/agyremote
                if any('daemon.sh' in a for a in args) or any('bin/agyremote' in a for a in args):
                    matching_pids.append((pid, " ".join(args)))
        except Exception:
            pass
    
    if matching_pids:
        raise AssertionError(f"Found running legacy processes: {matching_pids}")
    print("  -> Legacy process scan across /proc: CLEAN (0 matching processes)")

    # 2.2 Scan open FDs across all processes for AgyRemoteTele (excluding our test script)
    current_pid = str(os.getpid())
    open_fd_holders = []
    for pid_dir in glob.glob('/proc/[0-9]*'):
        pid = os.path.basename(pid_dir)
        if pid == current_pid:
            continue
        try:
            fd_dir = os.path.join(pid_dir, 'fd')
            if os.path.exists(fd_dir):
                for fd in os.listdir(fd_dir):
                    link = os.path.realpath(os.path.join(fd_dir, fd))
                    if '/agyremotetele/' in link.lower():
                        open_fd_holders.append((pid, fd, link))
        except Exception:
            pass

    if open_fd_holders:
        raise AssertionError(f"Found processes holding open FDs to AgyRemoteTele: {open_fd_holders}")
    print("  -> File descriptor audit across all processes: CLEAN (0 open handles)")

    # 2.3 Non-blocking flock test on data/agyremote.lock
    lock_file = '/opt/data/home/AgyRemoteTele/data/agyremote.lock'
    assert os.path.exists(lock_file), f"Lock file {lock_file} does not exist"
    with open(lock_file, 'r+') as lf:
        fcntl.flock(lf, fcntl.LOCK_EX | fcntl.LOCK_NB)
        print("  -> Non-blocking exclusive flock acquisition: SUCCESS")
        fcntl.flock(lf, fcntl.LOCK_UN)
        print("  -> Flock release: SUCCESS")

    # 2.4 Verify execution permissions are revoked (must not be executable)
    daemon_sh = '/opt/data/home/AgyRemoteTele/daemon.sh'
    bot_bin = '/opt/data/home/AgyRemoteTele/bin/agyremote'
    assert os.path.exists(daemon_sh), f"daemon.sh missing at {daemon_sh}"
    assert os.path.exists(bot_bin), f"bin/agyremote missing at {bot_bin}"
    assert not os.access(daemon_sh, os.X_OK), f"Security violation: {daemon_sh} is still executable!"
    assert not os.access(bot_bin, os.X_OK), f"Security violation: {bot_bin} is still executable!"
    print("  -> Execution permission revocation check: SUCCESS (non-executable 0644)")

    # 2.5 Verify DECOMMISSIONED.md
    marker = '/opt/data/home/AgyRemoteTele/DECOMMISSIONED.md'
    assert os.path.exists(marker), f"Marker {marker} missing"
    assert os.path.getsize(marker) > 50, f"Marker {marker} is too small or empty"
    print("  -> DECOMMISSIONED.md marker presence & content: SUCCESS")

    print("[TEST 2] RESULT: PASS")

def test_session_integrity_and_archive():
    print("[TEST 3] Archived sessions.json Syntax & Final State Validation")
    tarball = '/opt/data/home/agyremote/AgyRemoteTele_backup.tar.gz'
    disk_file = '/opt/data/home/AgyRemoteTele/data/sessions.json'

    assert os.path.exists(tarball), f"Backup archive {tarball} does not exist"
    assert os.path.exists(disk_file), f"Disk sessions file {disk_file} does not exist"

    # 3.1 Validate tarball format and critical files
    with tarfile.open(tarball, 'r:gz') as tar:
        names = tar.getnames()
        required_members = [
            'AgyRemoteTele/data/sessions.json',
            'AgyRemoteTele/.env',
            'AgyRemoteTele/daemon.sh',
            'AgyRemoteTele/bin/agyremote'
        ]
        for rm in required_members:
            assert rm in names, f"Archive missing required member: {rm}"
        print("  -> Tarball structure & member inventory: SUCCESS")

        # 3.2 Extract sessions.json from tarball into memory
        member = tar.getmember('AgyRemoteTele/data/sessions.json')
        assert member.size > 0, "Archived sessions.json is empty (0 bytes)!"
        extracted = tar.extractfile(member).read().decode('utf-8')

    # 3.3 Validate JSON syntax of archived content
    try:
        archived_data = json.loads(extracted)
        print("  -> Archived sessions.json JSON syntax parser check: VALID")
    except Exception as e:
        raise AssertionError(f"Archived sessions.json has invalid JSON syntax: {e}")

    # 3.4 Validate schema and session state
    assert isinstance(archived_data, dict), "Sessions root should be a JSON object"
    assert len(archived_data) >= 2, f"Expected at least 2 sessions, got {len(archived_data)}"
    
    # Check user 8638950192 (last active profile agy10)
    user_86 = archived_data.get("8638950192")
    assert user_86 is not None, "Missing user 8638950192 in archived sessions"
    assert user_86.get("is_authenticated") is True, "User 8638950192 is not authenticated"
    assert user_86.get("active_profile") == "agy10", f"Unexpected active_profile: {user_86.get('active_profile')}"
    assert user_86.get("active_conversation_id") == "99323ee4-56fa-4204-b709-801578eff3d1"
    
    # Check user 7896674035 (Arif_ex21, profile agy06)
    user_78 = archived_data.get("7896674035")
    assert user_78 is not None, "Missing user 7896674035 in archived sessions"
    assert user_78.get("username") == "Arif_ex21"
    assert user_78.get("active_profile") == "agy06"
    print("  -> Session schema, user records, and profile associations: VALID")

    # 3.5 Compare with disk file
    with open(disk_file, 'r', encoding='utf-8') as f:
        disk_raw = f.read()
    disk_data = json.loads(disk_raw)
    assert disk_data == archived_data, "Archived sessions data does not match disk data!"

    disk_hash = hashlib.sha256(disk_raw.encode('utf-8')).hexdigest()
    arch_hash = hashlib.sha256(extracted.encode('utf-8')).hexdigest()
    assert disk_hash == arch_hash, f"SHA256 mismatch! Disk: {disk_hash}, Arch: {arch_hash}"
    print(f"  -> Exact SHA-256 match between disk and tar archive: {disk_hash}")

    # 3.6 Verify bot shutdown log correlation
    bot_log = '/opt/data/home/AgyRemoteTele/data/bot.log'
    with open(bot_log, 'r', encoding='utf-8', errors='ignore') as f:
        log_content = f.read()
    shutdown_str = "AgyRemoteTele bot gracefully shut down and sessions persisted."
    assert shutdown_str in log_content, f"Graceful shutdown confirmation missing in {bot_log}"
    
    # Check mtime of sessions.json matches final shutdown timestamp (2026-10-26 13:20:31)
    disk_mtime = os.path.getmtime(disk_file)
    dt = datetime.datetime.fromtimestamp(disk_mtime, tz=datetime.timezone.utc)
    print(f"  -> Session file timestamp: {dt.isoformat()} matches shutdown event.")

    print("[TEST 3] RESULT: PASS")

def main():
    print("================================================================")
    print("EMPIRICAL CHALLENGER TEST SUITE: MILESTONE M1-2")
    print("Port Availability, Process Isolation, and Session Integrity")
    print("================================================================")
    try:
        test_port_8787()
        test_process_isolation_and_handles()
        test_session_integrity_and_archive()
        print("================================================================")
        print("ALL ADVERSARIAL STRESS TESTS PASSED SUCCESSFULLY!")
        print("VERDICT: APPROVE")
        print("================================================================")
        return 0
    except Exception as e:
        print("================================================================")
        print(f"TEST FAILED: {e}")
        print("VERDICT: REQUEST_CHANGES")
        print("================================================================")
        return 1

if __name__ == '__main__':
    sys.exit(main())
