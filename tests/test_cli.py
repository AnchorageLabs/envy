from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from envy.cli import cmd_backup, cmd_init, cmd_restore, load_settings, render_template


class Args:
    def __init__(self, **kwargs: object) -> None:
        self.__dict__.update(kwargs)


class EnvyCliTest(unittest.TestCase):
    def test_backup_restore_and_template(self) -> None:
        with tempfile.TemporaryDirectory() as repo_dir, tempfile.TemporaryDirectory() as home_dir:
            repo = Path(repo_dir)
            home = Path(home_dir)

            cmd_init(Args(repo=repo, home=home))
            (home / ".zshrc").write_text("export ENVY_TEST=1\n", encoding="utf-8")

            cmd_backup(Args(repo=repo, home=home, files=[]))
            self.assertEqual((repo / "home" / ".zshrc").read_text(encoding="utf-8"), "export ENVY_TEST=1\n")

            (home / ".zshrc").write_text("changed\n", encoding="utf-8")
            cmd_restore(Args(repo=repo, home=home, dry_run=False, force=True))
            self.assertEqual((home / ".zshrc").read_text(encoding="utf-8"), "export ENVY_TEST=1\n")

            settings = load_settings(Args(repo=repo, home=home))
            rendered = render_template("email={{ machine.email }} os={{ os }} env={{ env.ENVY_EMPTY }}", settings)
            self.assertIn("os=", rendered)
            self.assertIn("env=", rendered)


if __name__ == "__main__":
    unittest.main()
