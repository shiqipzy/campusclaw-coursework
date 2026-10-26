"""End-to-end check for lesson-four search. Run only against a disposable stack."""

import argparse
import json
import secrets
from pathlib import Path
from urllib.error import HTTPError
from urllib.parse import urlencode
from urllib.request import ProxyHandler, Request, build_opener


def read_env():
    return dict(
        line.split("=", 1)
        for line in Path(__file__).resolve().parents[1].joinpath(".env").read_text().splitlines()
        if "=" in line and not line.lstrip().startswith("#")
    )


def client(base, username, password):
    opener = build_opener(ProxyHandler({}))
    request = Request(
        base + "/api/login",
        json.dumps({"username": username, "password": password}).encode(),
        {"Content-Type": "application/json"},
    )
    with opener.open(request) as response:
        assert response.status == 200
        data = json.load(response)
        assert "Set-Cookie" not in response.headers
    opener.addheaders = [("Authorization", "Bearer " + data["token"])]
    return opener


def search(opener, base, phrase, **extra):
    with opener.open(base + "/api/search?" + urlencode({"q": phrase, **extra})) as response:
        assert response.status == 200
        return json.load(response)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://127.0.0.1:8080")
    args = parser.parse_args()
    base = args.base_url.rstrip("/")
    env = read_env()

    try:
        build_opener(ProxyHandler({})).open(base + "/api/search?" + urlencode({"q": "示例"}))
        raise AssertionError("anonymous search was allowed")
    except HTTPError as error:
        assert error.code == 401

    teacher = client(base, "teacher_a", env["TEACHER_A_PASSWORD"])
    student_a = client(base, "student_a1", env["STUDENT_A1_PASSWORD"])
    student_b = client(base, "student_b1", env["STUDENT_B1_PASSWORD"])

    assert search(student_a, base, "A班示例")["hits"]
    b_hit = search(student_b, base, "B班示例")["hits"][0]
    assert not search(student_a, base, "B班示例", class_id=2)["hits"]
    assert not search(student_b, base, "A班示例", class_id=1)["hits"]
    try:
        student_a.open(base + b_hit["material_url"])
        raise AssertionError("cross-class source URL was accessible")
    except HTTPError as error:
        assert error.code == 404
    absent = search(student_a, base, "天气预报")
    assert absent == {"hits": [], "message": "资料中未找到相关内容"}

    # Upload through the existing teacher-only API, then verify that the
    # student's hit points to exactly the same text and only the same class.
    boundary = "lesson4boundary"
    phrase = "紫色鲸鱼课表" + secrets.token_hex(4)
    body = f"# 检索验收\n{phrase} {boundary} 只对A班开放。\n".encode()
    multipart = (
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"search-check.md\"\r\n"
        "Content-Type: text/markdown\r\n\r\n"
    ).encode() + body + f"\r\n--{boundary}--\r\n".encode()
    with teacher.open(Request(base + "/api/materials", multipart, {"Content-Type": f"multipart/form-data; boundary={boundary}"})) as response:
        material_id = json.load(response)["id"]
        assert response.status == 201
    hits = search(student_a, base, phrase)["hits"]
    hit = next(hit for hit in hits if hit["material_id"] == material_id)
    assert hit["material_url"] == f"/api/materials/{material_id}"
    with student_a.open(base + hit["material_url"]) as response:
        material = json.load(response)
    assert material["content"][hit["char_start"] : hit["char_end"]].startswith(hit["excerpt"])
    assert not search(student_b, base, phrase, class_id=1)["hits"]
    question = Request(base + "/api/ask", json.dumps({"question": phrase}).encode(), {"Content-Type": "application/json"})
    with student_a.open(question) as response:
        answer = json.load(response)
    assert answer["sources"] and any(source["material_id"] == material_id for source in answer["sources"])
    assert answer["answer"]
    b_question = Request(base + "/api/ask", json.dumps({"question": phrase}).encode(), {"Content-Type": "application/json"})
    with student_b.open(b_question) as response:
        assert not json.load(response)["sources"]
    print("PASS: Bearer authentication, class isolation, source protection, markdown upload/search, and grounded answer")


if __name__ == "__main__":
    main()
