"""Execute oci-sync commands."""

import subprocess
from pathlib import Path

from .config import BINARY_PATH, TEST_REPO


def run_cmd(*args, check=True, cwd=None):
    """Run command, return result."""
    kwargs = {"capture_output": True, "text": True, "check": False}
    if cwd is not None:
        kwargs["cwd"] = cwd
    result = subprocess.run(args, **kwargs)
    if check and result.returncode != 0:
        raise RuntimeError(f"Command failed: {' '.join(args)}\n{result.stderr}")
    return result


def build_cmd(family: str, subcmd: str, *args):
    """Build oci-sync command based on family (standard or x)."""
    cmd = [str(BINARY_PATH)]
    if family == "x":
        cmd.append("x")
    cmd.extend([subcmd, *args])
    return cmd


def push(family: str, source_dir: Path, tag: str, passphrase: str = ""):
    """Push artifact to registry."""
    cmd = build_cmd(family, "push")
    if family == "standard":
        cmd.extend(["-l", str(source_dir), "-r", f"{TEST_REPO}:{tag}"])
    else:
        cmd.extend(["-l", str(source_dir), "--tag", tag])
    if passphrase:
        cmd.extend(["--passphrase", passphrase])
    run_cmd(*cmd, check=True)


def list_artifacts(family: str):
    """List artifacts in registry."""
    if family == "standard":
        return run_cmd(*build_cmd(family, "list", "-r", TEST_REPO), check=True)
    return run_cmd(*build_cmd(family, "list"), check=True)


def pull(family: str, tag: str, output_dir: Path, passphrase: str = ""):
    """Pull artifact from registry."""
    cmd = build_cmd(family, "pull")
    if family == "standard":
        cmd.extend(["-r", f"{TEST_REPO}:{tag}", "-l", str(output_dir)])
    else:
        cmd.extend(["--tag", tag, "-l", str(output_dir)])
    if passphrase:
        cmd.extend(["--passphrase", passphrase])
    run_cmd(*cmd, check=True)


def delete(family: str, tag: str):
    """Delete artifact from registry."""
    cmd = build_cmd(family, "delete")
    if family == "standard":
        cmd.extend(["-r", f"{TEST_REPO}:{tag}"])
    else:
        cmd.extend(["--tag", tag])
    run_cmd(*cmd, check=True)
