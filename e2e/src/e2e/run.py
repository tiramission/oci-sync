"""Execute oci-sync commands."""

import subprocess
import sh
from pathlib import Path
from dataclasses import dataclass

from .config import ROOT_DIR, BINARY_PATH, TEST_REPO


@dataclass
class CmdResult:
    stdout: str


_oci_sync = None


def _get_oci_sync():
    """Get oci-sync command, building binary first if needed."""
    global _oci_sync
    if _oci_sync is None:
        if not BINARY_PATH.exists():
            subprocess.run(
                ["go", "build", "-o", str(BINARY_PATH), "."],
                cwd=ROOT_DIR,
                check=True,
            )
        _oci_sync = sh.Command(BINARY_PATH)
    return _oci_sync


def run_cmd(*args, check=True, cwd=None):
    """Run command, return result."""
    kwargs = {}
    if cwd is not None:
        kwargs["_cwd"] = cwd
    try:
        result = _get_oci_sync()(*args, **kwargs)
        return CmdResult(stdout=str(result))
    except sh.ErrorReturnCode as e:
        if check:
            raise RuntimeError(f"Command failed: oci-sync {' '.join(args)}\n{e.stderr}")
        raise


def push(family: str, source_dir: Path, tag: str, passphrase: str = ""):
    """Push artifact to registry."""
    args = ["push"]
    if family == "standard":
        args.extend(["-l", str(source_dir), "-r", f"{TEST_REPO}:{tag}"])
    else:
        args[:1] = ["x", "push"]
        args.extend(["-l", str(source_dir), "--tag", tag])
    if passphrase:
        args.append(f"--passphrase={passphrase}")
    run_cmd(*args)


def list_artifacts(family: str):
    """List artifacts in registry."""
    if family == "standard":
        return run_cmd("list", "-r", TEST_REPO)
    return run_cmd("x", "list")


def pull(family: str, tag: str, output_dir: Path, passphrase: str = ""):
    """Pull artifact to registry."""
    args = ["pull"]
    if family == "standard":
        args.extend(["-r", f"{TEST_REPO}:{tag}", "-l", str(output_dir)])
    else:
        args[:1] = ["x", "pull"]
        args.extend(["--tag", tag, "-l", str(output_dir)])
    if passphrase:
        args.append(f"--passphrase={passphrase}")
    run_cmd(*args)


def delete(family: str, tag: str):
    """Delete artifact from registry."""
    args = ["delete"]
    if family == "standard":
        args.extend(["-r", f"{TEST_REPO}:{tag}"])
    else:
        args[:1] = ["x", "delete"]
        args.extend(["--tag", tag])
    run_cmd(*args)
