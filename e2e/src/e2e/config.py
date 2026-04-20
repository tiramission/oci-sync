"""Configuration and constants for e2e tests."""

import os
from pathlib import Path

ROOT_DIR = Path(__file__).parent.parent.parent.parent
WORK_DIR = ROOT_DIR / "e2e" / "runtime-check"
BINARY_PATH = WORK_DIR / "oci-sync"

TEST_REPO = os.getenv("OCI_SYNC_TEST_REPO", "internal.183867412.xyz:5000/file/share")
BASE_TAG = os.getenv("OCI_SYNC_TEST_TAG_BASE", f"runtime-check-{os.getpid()}")
ENCRYPTED_PASSPHRASE = os.getenv("OCI_SYNC_TEST_PASSPHRASE", "runtime-check-secret")
KEEP_WORKDIR = os.getenv("OCI_SYNC_KEEP_WORKDIR", "0") == "1"

TEST_CASES = [
    ("standard", "plain", f"{BASE_TAG}-standard-plain", ""),
    ("standard", "encrypted", f"{BASE_TAG}-standard-encrypted", ENCRYPTED_PASSPHRASE),
    ("x", "plain", f"{BASE_TAG}-x-plain", ""),
    ("x", "encrypted", f"{BASE_TAG}-x-encrypted", ENCRYPTED_PASSPHRASE),
]

TEST_FILES = {
    "hello.txt": "runtime-check-{}",
    "sub/nested.txt": "nested-content-{}",
}

REPO_NAME = TEST_REPO.rsplit("/", 1)[-1]
