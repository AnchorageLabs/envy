"""Tests for load_settings() config validation, including duplicate-files detection."""
from __future__ import annotations

import argparse
import textwrap
import unittest
from pathlib import Path
from tempfile import TemporaryDirectory

import sys
import os

# Ensure src/ is on the path when running via unittest discover with PYTHONPATH=src
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "src"))

from envy.cli import EnvyError, load_settings


def _make_args(repo: str, home: str) -> argparse.Namespace:
    ns = argparse.Namespace()
    ns.repo = repo
    ns.home = home
    return ns


def _write_config(repo_dir: Path, content: str) -> None:
    envy_dir = repo_dir / ".envy"
    envy_dir.mkdir(parents=True, exist_ok=True)
    (envy_dir / "config.toml").write_text(content, encoding="utf-8")


class TestLoadSettingsUniqueFiles(unittest.TestCase):
    """load_settings() succeeds when all files entries are unique."""

    def test_unique_files_loads_successfully(self) -> None:
        with TemporaryDirectory() as tmp:
            repo = Path(tmp)
            _write_config(
                repo,
                textwrap.dedent("""\
                    files = [".zshrc", ".gitconfig", ".vimrc"]
                """),
            )
            args = _make_args(str(repo), str(repo))
            settings = load_settings(args)
            self.assertEqual(settings.files, [".zshrc", ".gitconfig", ".vimrc"])

    def test_empty_files_loads_successfully(self) -> None:
        with TemporaryDirectory() as tmp:
            repo = Path(tmp)
            _write_config(
                repo,
                textwrap.dedent("""\
                    files = []
                """),
            )
            args = _make_args(str(repo), str(repo))
            settings = load_settings(args)
            self.assertEqual(settings.files, [])


class TestLoadSettingsDuplicateFiles(unittest.TestCase):
    """load_settings() raises EnvyError when files contains a duplicate entry."""

    def test_duplicate_entry_raises_envy_error(self) -> None:
        with TemporaryDirectory() as tmp:
            repo = Path(tmp)
            _write_config(
                repo,
                textwrap.dedent("""\
                    files = [".zshrc", ".gitconfig", ".zshrc"]
                """),
            )
            args = _make_args(str(repo), str(repo))
            with self.assertRaises(EnvyError) as ctx:
                load_settings(args)
            self.assertIn(".zshrc", str(ctx.exception))

    def test_duplicate_error_message_contains_path(self) -> None:
        with TemporaryDirectory() as tmp:
            repo = Path(tmp)
            _write_config(
                repo,
                textwrap.dedent("""\
                    files = [".bashrc", ".bashrc"]
                """),
            )
            args = _make_args(str(repo), str(repo))
            with self.assertRaises(EnvyError) as ctx:
                load_settings(args)
            self.assertIn(".bashrc", str(ctx.exception))
            self.assertIn("Duplicate", str(ctx.exception))

    def test_first_duplicate_is_reported(self) -> None:
        """When multiple duplicates exist, the first one encountered is reported."""
        with TemporaryDirectory() as tmp:
            repo = Path(tmp)
            _write_config(
                repo,
                textwrap.dedent("""\
                    files = [".zshrc", ".gitconfig", ".zshrc", ".gitconfig"]
                """),
            )
            args = _make_args(str(repo), str(repo))
            with self.assertRaises(EnvyError) as ctx:
                load_settings(args)
            # .zshrc appears as the first duplicate
            self.assertIn(".zshrc", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
