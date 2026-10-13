#!/usr/bin/env python3
"""Prepare only bounded public browser settings for the chat image build."""

import os
from pathlib import Path
import re
import sys
from typing import Mapping

INVALID_MESSAGE = "Invalid public chat browser configuration."
OUTPUT_DELIMITER = "CHAT_SENTRY_PUBLIC_BUILD_ARGS"
DSN_PATTERN = re.compile(
    r"https://[a-f0-9]{32}@o[0-9]+\.ingest(?:\.[a-z]+)?\.sentry\.io/[0-9]{1,20}"
)
ENVIRONMENT_PATTERN = re.compile(r"[A-Za-z0-9_-]{1,32}")
SHA_PATTERN = re.compile(r"[a-f0-9]{40}")


# PublicConfigurationError reports a fixed configuration failure without input values.
class PublicConfigurationError(ValueError):
    """PublicConfigurationError never contains submitted configuration values."""


# prepare_browser_build_args returns exactly four public arguments, blank when disabled.
def prepare_browser_build_args(environ: Mapping[str, str]) -> str:
    enabled = environ.get("CHAT_SENTRY_ENABLED", "") == "true"
    dsn = environment = release = ""
    if enabled:
        dsn = environ.get("CHAT_SENTRY_DSN", "")
        environment = environ.get("CHAT_SENTRY_ENVIRONMENT", "")
        source_sha = environ.get("CHAT_SENTRY_SOURCE_SHA", "")
        if (
            len(dsn) > 256
            or not DSN_PATTERN.fullmatch(dsn)
            or not ENVIRONMENT_PATTERN.fullmatch(environment)
            or not SHA_PATTERN.fullmatch(source_sha)
        ):
            raise PublicConfigurationError(INVALID_MESSAGE)
        release = "go-ramjet@" + source_sha
    return "\n".join(
        (
            "VITE_CHAT_SENTRY_ENABLED=" + ("true" if enabled else "false"),
            "VITE_CHAT_SENTRY_DSN=" + dsn,
            "VITE_CHAT_SENTRY_ENVIRONMENT=" + environment,
            "VITE_CHAT_SENTRY_RELEASE=" + release,
        )
    )


# write_actions_output appends the validated four arguments to the runner output file.
def write_actions_output(path: str, build_args: str) -> None:
    with Path(path).open("a", encoding="utf-8") as output:
        output.write("build_args<<" + OUTPUT_DELIMITER + "\n")
        output.write(build_args + "\n" + OUTPUT_DELIMITER + "\n")


# main prepares public arguments from approved names, returning one on a fixed failure.
def main() -> int:
    try:
        build_args = prepare_browser_build_args(os.environ)
        output_path = os.environ.get("GITHUB_OUTPUT", "")
        if not output_path:
            raise PublicConfigurationError(INVALID_MESSAGE)
        write_actions_output(output_path, build_args)
    except (PublicConfigurationError, OSError):
        print(INVALID_MESSAGE, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
