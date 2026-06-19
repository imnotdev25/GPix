#!/usr/bin/env python3
"""End-to-end smoke test for the gpix S3 gateway using a real S3 client (boto3).

It exercises the full SigV4 path: list-buckets, put, head, get (with a verified
round-trip), list (v2), range-get, and delete.

Start a server first. The quickest way (no Google credentials needed) is the
in-memory test harness:

    go run ./cmd/gpix-gateway-test -s3 127.0.0.1:9000 \\
        -access test -secret testsecret -bucket gpix

Then run:

    pip install boto3
    S3_ENDPOINT=http://127.0.0.1:9000 S3_ACCESS=test S3_SECRET=testsecret \\
        S3_BUCKET=gpix python3 test/s3_smoke.py

Against a live gpix, use the access key/secret you generated on the Connections
page.
"""
import os
import sys
import uuid

import boto3
from botocore.config import Config
from botocore.exceptions import ClientError

ENDPOINT = os.environ.get("S3_ENDPOINT", "http://127.0.0.1:9000")
ACCESS = os.environ.get("S3_ACCESS", "test")
SECRET = os.environ.get("S3_SECRET", "testsecret")
BUCKET = os.environ.get("S3_BUCKET", "gpix")


def main() -> int:
    s3 = boto3.client(
        "s3",
        endpoint_url=ENDPOINT,
        aws_access_key_id=ACCESS,
        aws_secret_access_key=SECRET,
        region_name="us-east-1",
        config=Config(s3={"addressing_style": "path"}, signature_version="s3v4"),
    )

    key = f"gpix-smoke-{uuid.uuid4().hex}.txt"
    payload = b"hello from the gpix s3 gateway\n" * 100  # ~3 KB
    failures = []

    def check(name, cond):
        status = "ok " if cond else "FAIL"
        print(f"[{status}] {name}")
        if not cond:
            failures.append(name)

    # list buckets
    try:
        buckets = [b["Name"] for b in s3.list_buckets().get("Buckets", [])]
        check("list_buckets contains bucket", BUCKET in buckets)
    except ClientError as e:
        check(f"list_buckets ({e})", False)

    # put
    try:
        s3.put_object(Bucket=BUCKET, Key=key, Body=payload, ContentType="text/plain")
        check("put_object", True)
    except ClientError as e:
        check(f"put_object ({e})", False)
        return _summary(failures)

    # head
    try:
        h = s3.head_object(Bucket=BUCKET, Key=key)
        check("head_object size matches", h["ContentLength"] == len(payload))
    except ClientError as e:
        check(f"head_object ({e})", False)

    # get + round-trip
    try:
        got = s3.get_object(Bucket=BUCKET, Key=key)["Body"].read()
        check("get_object round-trips", got == payload)
    except ClientError as e:
        check(f"get_object ({e})", False)

    # list v2 sees the key
    try:
        listed = s3.list_objects_v2(Bucket=BUCKET)
        keys = [o["Key"] for o in listed.get("Contents", [])]
        check("list_objects_v2 contains key", key in keys)
    except ClientError as e:
        check(f"list_objects_v2 ({e})", False)

    # range get (first 11 bytes)
    try:
        part = s3.get_object(Bucket=BUCKET, Key=key, Range="bytes=0-10")["Body"].read()
        check("ranged get returns 11 bytes", part == payload[:11])
    except ClientError as e:
        check(f"ranged get ({e})", False)

    # delete + confirm gone
    try:
        s3.delete_object(Bucket=BUCKET, Key=key)
        check("delete_object", True)
    except ClientError as e:
        check(f"delete_object ({e})", False)

    try:
        s3.head_object(Bucket=BUCKET, Key=key)
        check("object gone after delete", False)
    except ClientError as e:
        check("object gone after delete", e.response["Error"]["Code"] in ("404", "NoSuchKey", "NotFound"))

    return _summary(failures)


def _summary(failures) -> int:
    print("-" * 40)
    if failures:
        print(f"{len(failures)} check(s) FAILED: {', '.join(failures)}")
        return 1
    print("all checks passed ✓")
    return 0


if __name__ == "__main__":
    sys.exit(main())
