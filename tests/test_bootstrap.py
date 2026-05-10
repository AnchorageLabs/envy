"""Tests for `envy bootstrap` dry-run visibility and normal execution."""
from __future__ import annotations

import io
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch, MagicMock

# Ensure src/ is on the path when running via unittest discover with PYTHONPATH=src.
sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from envy.cli import main


MINIMAL_CONFIG = """files = []
"""


def _make_repo(tmp: Path, *, create_script: bool = False, make_executable: bool = False) -> Path:
    """Create a minimal envy repo under *tmp* and return its path."""
    repo = tmp / "repo"
    (repo / ".envy").mkdir(parents=True)
    (repo / "home").mkdir()
    (repo / "templates").mkdir()
    (repo / "scripts").mkdir()
    (repo / ".envy" / "config.toml").write_text(MINIMAL_CONFIG, encoding="utf-8")
    if create_script:
        script = repo / "scripts" / "bootstrap"
        script.write_text("#!/bin/sh\necho hello\n", encoding="utf-8")
        if make_executable:
            script.chmod(0o755)
        else:
            script.chmod(0o644)
    return repo


class TestBootstrapDryRunNoScript(unittest.TestCase):
    """Dry-run when scripts/bootstrap does not exist."""

    def test_no_script_message(self):
        with tempfile.TemporaryDirectory() as tmp_str:
            tmp = Path(tmp_str)
            repo = _make_repo(tmp)
            captured = io.StringIO()
            with patch("sys.stdout", captured):
                rc = main(["--repo", str(repo), "bootstrap", "--dry-run"])
            self.assertEqual(rc, 0)
            output = captured.getvalue()
            self.assertIn("not found", output)
            self.assertNotIn("would run", output)

    def test_no_subprocess_called(self):
        with tempfile.TemporaryDirectory() as tmp_str:
            tmp = Path(tmp_str)
            repo = _make_repo(tmp)
            with patch("envy.cli.run") as mock_run:
                main(["--repo", str(repo), "bootstrap", "--dry-run"])
                mock_run.assert_not_called()


class TestBootstrapDryRunNotExecutable(unittest.TestCase):
    """Dry-run when scripts/bootstrap exists but is not executable."""

    def test_not_executable_message(self):
        with tempfile.TemporaryDirectory() as tmp_str:
            tmp = Path(tmp_str)
            repo = _make_repo(tmp, create_script=True, make_executable=False)
            script_path = repo / "scripts" / "bootstrap"
            captured = io.StringIO()
            # Mock os.access so the test is not affected by running as root.
            with patch("envy.cli.os.access", return_value=False), \
                 patch("sys.stdout", captured):
                rc = main(["--repo", str(repo), "bootstrap", "--dry-run"])
            self.assertEqual(rc, 0)
            output = captured.getvalue()
            self.assertIn(str(script_path), output)
            self.assertIn("not executable", output)
            self.assertNotIn("would run", output)

    def test_no_subprocess_called(self):
        with tempfile.TemporaryDirectory() as tmp_str:
            tmp = Path(tmp_str)
            repo = _make_repo(tmp, create_script=True, make_executable=False)
            with patch("envy.cli.os.access", return_value=False), \
                 patch("envy.cli.run") as mock_run:
                main(["--repo", str(repo), "bootstrap", "--dry-run"])
                mock_run.assert_not_called()


class TestBootstrapDryRunExecutable(unittest.TestCase):
    """Dry-run when scripts/bootstrap exists and is executable."""

    def test_executable_message(self):
        with tempfile.TemporaryDirectory() as tmp_str:
            tmp = Path(tmp_str)
            repo = _make_repo(tmp, create_script=True, make_executable=True)
            script_path = repo / "scripts" / "bootstrap"
            captured = io.StringIO()
            with patch("envy.cli.os.access", return_value=True), \
                 patch("sys.stdout", captured):
                rc = main(["--repo", str(repo), "bootstrap", "--dry-run"])
            self.assertEqual(rc, 0)
            output = captured.getvalue()
            self.assertIn(str(script_path), output)
            self.assertIn("executable", output)
            self.assertIn("would run", output)

    def test_no_subprocess_called(self):
        with tempfile.TemporaryDirectory() as tmp_str:
            tmp = Path(tmp_str)
            repo = _make_repo(tmp, create_script=True, make_executable=True)
            with patch("envy.cli.os.access", return_value=True), \
                 patch("envy.cli.run") as mock_run:
                main(["--repo", str(repo), "bootstrap", "--dry-run"])
                mock_run.assert_not_called()


class TestBootstrapNormalExecution(unittest.TestCase):
    """Normal (non-dry-run) bootstrap behavior is unchanged."""

    def test_script_executed_when_present(self):
        with tempfile.TemporaryDirectory() as tmp_str:
            tmp = Path(tmp_str)
            repo = _make_repo(tmp, create_script=True, make_executable=True)
            script_path = repo / "scripts" / "bootstrap"
            with patch("envy.cli.run") as mock_run:
                rc = main(["--repo", str(repo), "bootstrap"])
            self.assertEqual(rc, 0)
            mock_run.assert_called_once_with([str(script_path)], cwd=repo)

    def test_no_script_executed_when_absent(self):
        with tempfile.TemporaryDirectory() as tmp_str:
            tmp = Path(tmp_str)
            repo = _make_repo(tmp, create_script=False)
            with patch("envy.cli.run") as mock_run:
                rc = main(["--repo", str(repo), "bootstrap"])
            self.assertEqual(rc, 0)
            mock_run.assert_not_called()


if __name__ == "__main__":
    unittest.main()
