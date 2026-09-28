"""Run HTTP checks against this project's local Compose stack only.

Uses .env for demo passwords. Prints no passwords or full session values.
"""

import http.client
import json
import secrets
from pathlib import Path

BASE = Path(__file__).resolve().parent.parent
PASSWORDS = dict(line.split("=", 1) for line in (BASE / ".env").read_text(encoding="utf-8-sig").splitlines() if "=" in line)
PORT = int(PASSWORDS.get("WEB_PORT", "8080"))


def request(method, path, body=None, headers=None, cookie=None):
    conn = http.client.HTTPConnection("localhost", PORT, timeout=30)
    h = dict(headers or {})
    if cookie:
        h["Cookie"] = cookie
    conn.request(method, path, body=body, headers=h)
    response = conn.getresponse()
    data = response.read()
    result = (response.status, dict(response.getheaders()), data)
    conn.close()
    return result


def parsed(result):
    return json.loads(result[2].decode("utf-8"))


def login(username, password):
    result = request("POST", "/api/login", json.dumps({"username": username, "password": password}).encode(), {"Content-Type": "application/json"})
    assert result[0] == 200, (username, result[0], result[2])
    cookie = result[1]["Set-Cookie"].split(";", 1)[0]
    return result, cookie


def multipart(filename, content, extras=""):
    boundary = "CampusClaw" + secrets.token_hex(12)
    body = (f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"{filename}\"\r\nContent-Type: application/octet-stream\r\n\r\n".encode()
            + content + f"\r\n--{boundary}--\r\n".encode())
    return body, {"Content-Type": f"multipart/form-data; boundary={boundary}"}


def main():
    checks = {}
    assert request("GET", "/health")[0] == 200
    assert request("GET", "/api/me")[0] == 401
    checks["health_and_unauthorized_me"] = "200 / 401"

    teacher_result, teacher = login("teacher_a", PASSWORDS["TEACHER_A_PASSWORD"])
    student_a_result, student_a = login("student_a1", PASSWORDS["STUDENT_A1_PASSWORD"])
    _, student_b = login("student_b1", PASSWORDS["STUDENT_B1_PASSWORD"])
    user = parsed(teacher_result)
    assert user["role"] == "teacher" and user["class_name"] == "A班"
    me = request("GET", "/api/me", cookie=teacher)
    assert me[0] == 200 and parsed(me)["class_id"] == 1
    checks["login_and_session"] = "200; HttpOnly/Lax cookie; /api/me class A"

    wrong = request("POST", "/api/login", b'{"username":"nonexistent","password":"wrong"}', {"Content-Type": "application/json"})
    assert wrong[0] == 401
    checks["bad_credentials"] = "401"

    a_list = parsed(request("GET", "/api/materials", cookie=student_a))
    b_list = parsed(request("GET", "/api/materials", cookie=student_b))
    assert a_list and b_list and all(m["class_id"] == 1 for m in a_list) and all(m["class_id"] == 2 for m in b_list)
    a_id = next(m["id"] for m in a_list if m["filename"] == "A班示例.txt")
    b_id = next(m["id"] for m in b_list if m["filename"] == "B班示例.txt")
    assert request("GET", f"/api/materials/{b_id}", cookie=student_a)[0] == 404
    assert request("GET", f"/api/materials/{b_id}/file", cookie=student_a)[0] == 404
    assert request("GET", f"/api/materials/{a_id}/file", cookie=student_a)[0] == 200
    assert request("GET", f"/api/materials/{a_id}/file")[0] == 401
    assert request("GET", "/uploads/seed-class-1.txt")[0] == 404
    checks["class_and_download"] = "same class 200; cross class 404; anonymous 401; static 404"

    before = len(a_list)
    ids = []
    for filename, content in [("验收文本.txt", b"A class text\n"), ("验收Markdown.md", "# 标题\n<script>alert(1)</script>\n".encode())]:
        body, headers = multipart(filename, content)
        status, _, data = request("POST", "/api/materials", body, headers, teacher)
        assert status == 201, (status, data)
        m = json.loads(data)
        ids.append(m["id"])
        detail = parsed(request("GET", f"/api/materials/{m['id']}", cookie=student_a))
        assert detail["content"] == content.decode()
        assert request("GET", f"/api/materials/{m['id']}/file", cookie=student_a)[2] == content
    checks["txt_md_upload_and_student_read"] = f"201 / 201; IDs {ids}; content and download match"

    for filename, content, expected in [
        ("wrong.pdf", b"PDF", 400), ("empty.txt", b"", 400), ("invalid.md", b"\xff", 400),
        ("huge.txt", b"x" * (5 * 1024 * 1024 + 1), 413),
    ]:
        body, headers = multipart(filename, content)
        result = request("POST", "/api/materials", body, headers, teacher)
        assert result[0] == expected, (filename, result[0], result[2][:100])
    body, headers = multipart("student.txt", b"student upload")
    assert request("POST", "/api/materials", body, headers, student_a)[0] == 403
    final_list = parsed(request("GET", "/api/materials", cookie=teacher))
    assert len(final_list) == before + 2
    checks["invalid_and_student_upload"] = "wrong/empty/UTF-8 400; oversize 413; student 403; no extra records"

    logout = request("POST", "/api/logout", cookie=student_a)
    assert logout[0] == 204 and request("GET", "/api/me", cookie=student_a)[0] == 401
    checks["logout"] = "204; old cookie 401"

    redacted = dict(teacher_result[1])
    redacted["Set-Cookie"] = redacted["Set-Cookie"].replace(teacher.split("=", 1)[1], "<redacted>")
    evidence = {"request": {"method": "POST", "url": f"http://localhost:{PORT}/api/login", "json": {"username": "teacher_a", "password": "<redacted>"}},
                "response": {"status": teacher_result[0], "headers": redacted, "body": parsed(teacher_result)}, "checks": checks}
    (BASE / "deliverables" / "http-evidence.json").write_text(json.dumps(evidence, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(checks, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
