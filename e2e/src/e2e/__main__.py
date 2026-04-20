#!/usr/bin/env python3
"""
E2E runtime check for oci-sync.
Usage: uv run -m e2e

Environment variables:
  OCI_SYNC_TEST_REPO: repository URL (default: internal.183867412.xyz:5000/file/share)
  OCI_SYNC_TEST_TAG_BASE: base tag prefix (default: runtime-check-<pid>)
  OCI_SYNC_TEST_PASSPHRASE: encryption passphrase (default: runtime-check-secret)
  OCI_SYNC_KEEP_WORKDIR: keep work directory for debugging (default: 0)
"""

import sys
import atexit
from concurrent.futures import ThreadPoolExecutor, as_completed

from rich.console import Console
from rich.table import Table

from .config import TEST_CASES, REPO_NAME, BASE_TAG
from .env import build_binary, setup_shortcut_config, cleanup_workdir
from .case import run_case
from .state import console, get_errors, incr_errors

atexit.register(cleanup_workdir)


def main():
    build_binary()
    setup_shortcut_config()

    results = {}
    with ThreadPoolExecutor(max_workers=4) as executor:
        futures = {executor.submit(run_case, *case): case for case in TEST_CASES}
        for future in as_completed(futures):
            case = futures[future]
            try:
                success = future.result()
                results[case] = success
            except Exception as e:
                console.print(f"[red]Case {case} failed: {e}[/red]")
                results[case] = False
                incr_errors()

    print_results(results)

    if get_errors() > 0:
        console.print(f"\n[bold red]{get_errors()} test(s) failed![/bold red]")
        sys.exit(1)

    tags = f"{BASE_TAG}-standard-plain, {BASE_TAG}-standard-encrypted, {BASE_TAG}-x-plain, {BASE_TAG}-x-encrypted"
    console.print("\n[bold green]All runtime checks passed![/bold green]")
    console.print(f"Tags: {tags}")


def print_results(results):
    table = Table(title="Results")
    table.add_column("Case", style="cyan")
    table.add_column("Status", style="green")

    for case, success in results.items():
        status = "PASS" if success else "FAIL"
        case_str = f"{case[0]}/{case[1]}"
        if success:
            table.add_row(case_str, f"[green]{status}[/green]")
        else:
            table.add_row(case_str, f"[red]{status}[/red]")

    console.print()
    console.print(table)


if __name__ == "__main__":
    main()
