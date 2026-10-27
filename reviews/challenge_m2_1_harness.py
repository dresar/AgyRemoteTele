import json
import time
import urllib.request
import urllib.error
import urllib.parse
from concurrent.futures import ThreadPoolExecutor, as_completed

BASE_URL = "http://127.0.0.1:8787"
VALID_AUTH = "Bearer agy-cluster-control-2026"

def request_http(url, method="GET", headers=None, data=None):
    if headers is None:
        headers = {}
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    start_time = time.time()
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            body = resp.read().decode("utf-8")
            elapsed = time.time() - start_time
            return resp.status, body, resp.headers, elapsed
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8")
        elapsed = time.time() - start_time
        return e.code, body, e.headers, elapsed
    except Exception as e:
        elapsed = time.time() - start_time
        return 0, str(e), {}, elapsed

def stress_health_concurrent(num_requests=250, max_workers=30):
    url = f"{BASE_URL}/api/health"
    results = []
    with ThreadPoolExecutor(max_workers=max_workers) as executor:
        futures = [executor.submit(request_http, url, "GET") for _ in range(num_requests)]
        for f in as_completed(futures):
            results.append(f.result())
    
    status_codes = [r[0] for r in results]
    latencies = [r[3] for r in results]
    
    assert all(code == 200 for code in status_codes), f"Status codes mismatch: {set(status_codes)}"
    
    for _, body, _, _ in results:
        data = json.loads(body)
        assert data.get("status") == "ok", f"Invalid body: {data}"
        assert "uptime" in data
        assert "timestamp" in data
        assert data.get("version") == "1.0.0"
        
    avg_latency = sum(latencies) / len(latencies)
    max_latency = max(latencies)
    min_latency = min(latencies)
    return {
        "total": num_requests,
        "success_rate": 100.0,
        "min_ms": round(min_latency * 1000, 2),
        "avg_ms": round(avg_latency * 1000, 2),
        "max_ms": round(max_latency * 1000, 2)
    }

def test_health_methods():
    methods = ["POST", "PUT", "DELETE", "PATCH"]
    for m in methods:
        status, body, _, _ = request_http(f"{BASE_URL}/api/health", method=m)
        assert status == 405, f"Expected 405 for {m}, got {status}: {body}"
        data = json.loads(body)
        assert data.get("error") == "Method Not Allowed"

def test_bearer_boundaries():
    protected_paths = [
        "/api/workers",
        "/api/system/vitals",
        "/api/processes"
    ]
    
    for path in protected_paths:
        status, _, _, _ = request_http(f"{BASE_URL}{path}", "GET")
        assert status == 401, f"Missing auth on {path} returned {status}"
        
        status, _, _, _ = request_http(f"{BASE_URL}{path}", "GET", {"Authorization": ""})
        assert status == 401, f"Empty auth header on {path} returned {status}"
        
        status, _, _, _ = request_http(f"{BASE_URL}{path}", "GET", {"Authorization": "   "})
        assert status == 401, f"Whitespace auth header on {path} returned {status}"
        
        invalid_schemes = [
            "Basic dXNlcjpwYXNz",
            "Digest username=\"MIME\"",
            "Token agy-cluster-control-2026",
            "OAuth agy-cluster-control-2026",
            "Bearer",
            "Bearer ",
            "bearer   ",
            "CustomScheme 123"
        ]
        for scheme in invalid_schemes:
            status, _, _, _ = request_http(f"{BASE_URL}{path}", "GET", {"Authorization": scheme})
            assert status == 401, f"Scheme {scheme} on {path} returned {status}"
            
        corrupt_tokens = [
            "Bearer invalid-key-12345",
            "Bearer null",
            "Bearer undefined",
            "Bearer [object Object]",
            "Bearer 123456",
            "Bearer false"
        ]
        for tok in corrupt_tokens:
            status, _, _, _ = request_http(f"{BASE_URL}{path}", "GET", {"Authorization": tok})
            assert status == 401, f"Corrupt token {tok} on {path} returned {status}"

def test_oversized_bearer():
    path = "/api/workers"
    results = {}
    oversized_lengths = [4096, 5000, 8192, 16384]
    for length in oversized_lengths:
        header_val = "Bearer " + ("A" * length)
        status, body, _, _ = request_http(f"{BASE_URL}{path}", "GET", {"Authorization": header_val})
        assert status in [401, 431], f"Oversized {length} returned unexpected status {status}: {body}"
        results[str(length)] = status
    return results

def test_injection_auth_headers():
    path = "/api/workers"
    injections = [
        "Bearer ' OR '1'='1",
        "Bearer '; DROP TABLE users; --",
        "Bearer admin'--",
        "Bearer 1' UNION SELECT null, null, null--",
        "Bearer $(whoami)",
        "Bearer `whoami`",
        "Bearer ; id ;",
        "Bearer | id",
        "Bearer & id",
        "Bearer && id",
        "Bearer ${IFS}cat${IFS}/etc/passwd",
        "Bearer %s%s%s%s%s",
        "Bearer ../../../etc/passwd",
        "Bearer <script>alert(1)</script>",
        "Bearer \\x00admin"
    ]
    for inj in injections:
        status, body, _, _ = request_http(f"{BASE_URL}{path}", "GET", {"Authorization": inj})
        assert status == 401, f"Injection {inj} returned {status}: {body}"
        assert "error" in json.loads(body)

def test_valid_auth():
    paths = [
        "/api/workers",
        "/api/system/vitals",
        "/api/processes"
    ]
    for path in paths:
        status, body, _, _ = request_http(f"{BASE_URL}{path}", "GET", {"Authorization": VALID_AUTH})
        assert status == 200, f"Valid auth on {path} failed with {status}: {body}"

def test_public_routes_bypass_auth():
    public_endpoints = [
        "/api/health",
        "/"
    ]
    for p in public_endpoints:
        status, body, _, _ = request_http(f"{BASE_URL}{p}", "GET")
        assert status == 200, f"Public endpoint {p} without auth returned {status}"

def test_route_isolation():
    unprotected_unknown_routes = [
        "/api/unknown",
        "/api/v99/test",
        "/api/health/subroute",
        "/api/nonexistent_prefix_123"
    ]
    for r in unprotected_unknown_routes:
        status_unauth, _, _, _ = request_http(f"{BASE_URL}{r}", "GET")
        assert status_unauth == 404, f"Unauthenticated unknown route {r} returned {status_unauth}"
        
        status_auth, _, _, _ = request_http(f"{BASE_URL}{r}", "GET", {"Authorization": VALID_AUTH})
        assert status_auth == 404, f"Authenticated unknown route {r} returned {status_auth}"

    protected_unknown_routes = [
        "/api/system/nonexistent",
        "/api/processes/unknown/sub"
    ]
    for r in protected_unknown_routes:
        status_unauth, _, _, _ = request_http(f"{BASE_URL}{r}", "GET")
        assert status_unauth == 401, f"Unauthenticated protected subroute {r} returned {status_unauth}"
        
        status_auth, _, _, _ = request_http(f"{BASE_URL}{r}", "GET", {"Authorization": VALID_AUTH})
        assert status_auth == 404, f"Authenticated protected unknown route {r} returned {status_auth}"

def test_input_guards_kill():
    invalid_pids = ["0", "1", "2", "-1", "abc", "3.14", ";ls"]
    for pid in invalid_pids:
        status, body, _, _ = request_http(
            f"{BASE_URL}/api/processes/{urllib.parse.quote(pid)}/kill",
            "POST",
            {"Authorization": VALID_AUTH}
        )
        assert status in [400, 403], f"Invalid PID {pid} returned {status}: {body}"

def test_input_guards_workers():
    invalid_ids = ["agy99", "agy00", "agy01;ls", "agy01$foo", "agy01|bar"]
    for wid in invalid_ids:
        status, body, _, _ = request_http(
            f"{BASE_URL}/api/workers/{urllib.parse.quote(wid)}",
            "GET",
            {"Authorization": VALID_AUTH}
        )
        assert status in [400, 404], f"Invalid worker {wid} returned {status}: {body}"

def test_malformed_json_body():
    url = f"{BASE_URL}/api/workers/agy01/prompt"
    status, body, _, _ = request_http(
        url,
        "POST",
        {"Authorization": VALID_AUTH, "Content-Type": "application/json"},
        data=b"{\"prompt\": unclosed string"
    )
    assert status == 400, f"Malformed JSON returned {status}: {body}"

def test_empty_prompt():
    url = f"{BASE_URL}/api/workers/agy01/prompt"
    status, body, _, _ = request_http(
        url,
        "POST",
        {"Authorization": VALID_AUTH, "Content-Type": "application/json"},
        data=b"{\"prompt\": \"   \"}"
    )
    assert status == 400, f"Empty prompt returned {status}: {body}"

def main():
    report = {}
    print("Executing stress_health_concurrent (250 requests)...")
    report["stress_health"] = stress_health_concurrent(num_requests=250, max_workers=30)
    print("Completed stress_health_concurrent:", report["stress_health"])
    
    print("Executing test_health_methods...")
    test_health_methods()
    report["health_methods"] = "PASS"
    print("Completed test_health_methods.")
    
    print("Executing test_bearer_boundaries...")
    test_bearer_boundaries()
    report["bearer_boundaries"] = "PASS"
    print("Completed test_bearer_boundaries.")
    
    print("Executing test_oversized_bearer...")
    report["oversized_bearer"] = test_oversized_bearer()
    print("Completed test_oversized_bearer:", report["oversized_bearer"])
    
    print("Executing test_injection_auth_headers...")
    test_injection_auth_headers()
    report["injection_auth_headers"] = "PASS"
    print("Completed test_injection_auth_headers.")
    
    print("Executing test_valid_auth...")
    test_valid_auth()
    report["valid_auth"] = "PASS"
    print("Completed test_valid_auth.")
    
    print("Executing test_public_routes_bypass_auth...")
    test_public_routes_bypass_auth()
    report["public_routes"] = "PASS"
    print("Completed test_public_routes_bypass_auth.")
    
    print("Executing test_route_isolation...")
    test_route_isolation()
    report["route_isolation"] = "PASS"
    print("Completed test_route_isolation.")
    
    print("Executing test_input_guards_kill...")
    test_input_guards_kill()
    report["input_guards_kill"] = "PASS"
    print("Completed test_input_guards_kill.")
    
    print("Executing test_input_guards_workers...")
    test_input_guards_workers()
    report["input_guards_workers"] = "PASS"
    print("Completed test_input_guards_workers.")
    
    print("Executing test_malformed_json_body...")
    test_malformed_json_body()
    report["malformed_json_body"] = "PASS"
    print("Completed test_malformed_json_body.")
    
    print("Executing test_empty_prompt...")
    test_empty_prompt()
    report["empty_prompt"] = "PASS"
    print("Completed test_empty_prompt.")
    
    print(json.dumps(report, indent=2))
    print("ALL_ADVERSARIAL_CHALLENGES_PASSED")

if __name__ == "__main__":
    main()
