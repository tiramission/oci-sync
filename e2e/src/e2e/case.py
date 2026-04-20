"""Run a single test case."""

from pathlib import Path

from .config import TEST_REPO, WORK_DIR
from .run import push, list_artifacts, pull, delete
from .env import prepare_test_data
from .state import console


def run_case(family: str, case_name: str, tag: str, passphrase: str = "") -> bool:
    """Run a single test case and return success status."""
    remote_ref = f"{TEST_REPO}:{tag}"
    source_dir = WORK_DIR / f"{family}-{case_name}-src"
    output_dir = WORK_DIR / f"{family}-{case_name}-out"

    console.print(f"\n[bold]=== {family}/{case_name} ===[/bold]")

    prepare_test_data(source_dir, tag)

    # Push
    console.print("  Pushing...")
    push(family, source_dir, tag, passphrase)
    console.print(f"  [green]Pushed to {remote_ref}[/green]")

    # List
    console.print("  Listing...")
    result = list_artifacts(family)
    if tag not in result.stdout:
        raise RuntimeError(f"Tag {tag} not found in list output")
    console.print(f"  [green]Found tag {tag}[/green]")

    # Pull
    console.print("  Pulling...")
    output_dir.mkdir(parents=True, exist_ok=True)
    pull(family, tag, output_dir, passphrase)
    console.print(f"  [green]Pulled to {output_dir}[/green]")

    # Validate
    console.print("  Validating...")
    validate_content(source_dir, output_dir)
    console.print("  [green]Content validated[/green]")

    # Delete
    console.print("  Deleting...")
    delete(family, tag)
    console.print(f"  [green]Deleted {remote_ref}[/green]")

    console.print(f"  [bold green]✓ {family}/{case_name} passed[/bold green]")
    return True


def validate_content(source_dir: Path, output_dir: Path):
    """Validate pulled content matches original."""
    from .config import TEST_FILES

    actual_dir = output_dir / source_dir.name
    for path in TEST_FILES:
        assert (actual_dir / path).exists(), f"{path} not found"
        expected = (source_dir / path).read_text()
        actual = (actual_dir / path).read_text()
        assert actual == expected, f"{path} content mismatch"
