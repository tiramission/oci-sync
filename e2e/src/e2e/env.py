"""Build and setup test environment."""

import shutil
from pathlib import Path

from .config import ROOT_DIR, WORK_DIR, BINARY_PATH, TEST_REPO, TEST_FILES
from .run import run_cmd
from .state import console, get_errors


def build_binary():
    """Build oci-sync binary."""
    console.print("Building oci-sync binary...")
    WORK_DIR.mkdir(parents=True, exist_ok=True)
    run_cmd("go", "build", "-o", str(BINARY_PATH), ".", cwd=ROOT_DIR, check=True)
    console.print(f"[green]Built to {BINARY_PATH}[/green]")


def setup_shortcut_config():
    """Create shortcut config for x family commands."""
    console.print("Setting up shortcut config...")
    config = f"""shortcuts:
  x:
    repo: {TEST_REPO}
"""
    (WORK_DIR / "oci-sync.yaml").write_text(config)


def prepare_test_data(source_dir: Path, marker: str):
    """Create test data in source directory."""
    shutil.rmtree(source_dir, ignore_errors=True)
    source_dir.mkdir(parents=True, exist_ok=True)
    (source_dir / "sub").mkdir()
    for path, template in TEST_FILES.items():
        (source_dir / path).write_text(template.format(marker) + "\n")


def cleanup_workdir():
    """Clean up work directory based on test results."""
    from .config import KEEP_WORKDIR, WORK_DIR

    if KEEP_WORKDIR:
        console.print(
            f"[yellow]Keeping work directory {WORK_DIR} because OCI_SYNC_KEEP_WORKDIR=1[/yellow]"
        )
        return
    if get_errors() == 0:
        shutil.rmtree(WORK_DIR, ignore_errors=True)
    else:
        console.print(
            f"[yellow]Keeping work directory {WORK_DIR} for debugging because errors occurred[/yellow]"
        )
