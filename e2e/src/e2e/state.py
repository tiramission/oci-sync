"""Global test state."""

from rich.console import Console

errors = [0]
console = Console()


def incr_errors():
    errors[0] += 1


def get_errors():
    return errors[0]
