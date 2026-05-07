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


class RestoreBackupTest(unittest.TestCase):
    """Tests for the backup-on-force behaviour of `envy restore --force`."""

    def _setup(self, repo: Path, home: Path) -> None:
        """Initialise a minimal envy repo with .zshrc tracked."""
        cmd_init(Args(repo=repo, home=home))
        # Write the stored version of .zshrc into the repo
        (repo / "home" / ".zshrc").write_text("# stored\n", encoding="utf-8")
        # Overwrite config to track only .zshrc
        (repo / ".envy" / "config.toml").write_text(
            'files = [".zshrc"]\n', encoding="utf-8"
        )

    def test_existing_file_is_backed_up_on_force(self) -> None:
        """An existing target file is copied to .envy/backups/<ts>/ before overwrite."""
        with tempfile.TemporaryDirectory() as repo_dir, tempfile.TemporaryDirectory() as home_dir:
            repo = Path(repo_dir)
            home = Path(home_dir)
            self._setup(repo, home)

            original_content = "# original\n"
            (home / ".zshrc").write_text(original_content, encoding="utf-8")

            cmd_restore(Args(repo=repo, home=home, dry_run=False, force=True))

            # The target should now contain the stored version
            self.assertEqual((home / ".zshrc").read_text(encoding="utf-8"), "# stored\n")

            # A backup directory should exist under .envy/backups/
            backups_root = repo / ".envy" / "backups"
            self.assertTrue(backups_root.exists(), "backups directory should be created")
            timestamp_dirs = list(backups_root.iterdir())
            self.assertEqual(len(timestamp_dirs), 1, "exactly one timestamped backup dir expected")

            backup_file = timestamp_dirs[0] / ".zshrc"
            self.assertTrue(backup_file.exists(), "backed-up file should exist")
            self.assertEqual(backup_file.read_text(encoding="utf-8"), original_content)

    def test_existing_directory_is_backed_up_on_force(self) -> None:
        """An existing target directory is copied to .envy/backups/<ts>/ before overwrite."""
        with tempfile.TemporaryDirectory() as repo_dir, tempfile.TemporaryDirectory() as home_dir:
            repo = Path(repo_dir)
            home = Path(home_dir)
            cmd_init(Args(repo=repo, home=home))

            # Track a directory entry
            (repo / ".envy" / "config.toml").write_text(
                'files = [".myconf"]\n', encoding="utf-8"
            )
            # Stored version is a directory with one file
            stored_dir = repo / "home" / ".myconf"
            stored_dir.mkdir(parents=True, exist_ok=True)
            (stored_dir / "settings.ini").write_text("[new]\n", encoding="utf-8")

            # Existing target directory in home
            target_dir = home / ".myconf"
            target_dir.mkdir()
            (target_dir / "old.ini").write_text("[old]\n", encoding="utf-8")

            cmd_restore(Args(repo=repo, home=home, dry_run=False, force=True))

            # Target should now contain the stored version
            self.assertTrue((home / ".myconf" / "settings.ini").exists())
            self.assertFalse((home / ".myconf" / "old.ini").exists())

            # Backup should contain the old directory
            backups_root = repo / ".envy" / "backups"
            timestamp_dirs = list(backups_root.iterdir())
            self.assertEqual(len(timestamp_dirs), 1)
            backup_old = timestamp_dirs[0] / ".myconf" / "old.ini"
            self.assertTrue(backup_old.exists(), "old directory contents should be backed up")
            self.assertEqual(backup_old.read_text(encoding="utf-8"), "[old]\n")

    def test_missing_target_skips_backup(self) -> None:
        """When the target does not exist, no backup is created and restore succeeds."""
        with tempfile.TemporaryDirectory() as repo_dir, tempfile.TemporaryDirectory() as home_dir:
            repo = Path(repo_dir)
            home = Path(home_dir)
            self._setup(repo, home)

            # Ensure target does NOT exist
            target = home / ".zshrc"
            self.assertFalse(target.exists())

            cmd_restore(Args(repo=repo, home=home, dry_run=False, force=True))

            # File should be restored
            self.assertEqual(target.read_text(encoding="utf-8"), "# stored\n")

            # No backup directory should be created
            backups_root = repo / ".envy" / "backups"
            if backups_root.exists():
                timestamp_dirs = list(backups_root.iterdir())
                self.assertEqual(
                    len(timestamp_dirs), 0,
                    "no backup dirs should be created when target was absent"
                )

    def test_dry_run_does_not_create_backup(self) -> None:
        """--dry-run must not create any backup even when the target exists."""
        with tempfile.TemporaryDirectory() as repo_dir, tempfile.TemporaryDirectory() as home_dir:
            repo = Path(repo_dir)
            home = Path(home_dir)
            self._setup(repo, home)

            original_content = "# original\n"
            (home / ".zshrc").write_text(original_content, encoding="utf-8")

            cmd_restore(Args(repo=repo, home=home, dry_run=True, force=True))

            # Target should be unchanged
            self.assertEqual((home / ".zshrc").read_text(encoding="utf-8"), original_content)

            # No backup directory should be created
            backups_root = repo / ".envy" / "backups"
            self.assertFalse(
                backups_root.exists(),
                "backups directory must not be created during dry-run"
            )

    def test_all_backups_share_same_timestamp_dir(self) -> None:
        """All files backed up in a single restore run share one timestamp directory."""
        with tempfile.TemporaryDirectory() as repo_dir, tempfile.TemporaryDirectory() as home_dir:
            repo = Path(repo_dir)
            home = Path(home_dir)
            cmd_init(Args(repo=repo, home=home))

            # Track two files
            (repo / ".envy" / "config.toml").write_text(
                'files = [".zshrc", ".bashrc"]\n', encoding="utf-8"
            )
            (repo / "home" / ".zshrc").write_text("# stored zshrc\n", encoding="utf-8")
            (repo / "home" / ".bashrc").write_text("# stored bashrc\n", encoding="utf-8")

            # Both exist in home
            (home / ".zshrc").write_text("# old zshrc\n", encoding="utf-8")
            (home / ".bashrc").write_text("# old bashrc\n", encoding="utf-8")

            cmd_restore(Args(repo=repo, home=home, dry_run=False, force=True))

            backups_root = repo / ".envy" / "backups"
            timestamp_dirs = list(backups_root.iterdir())
            self.assertEqual(
                len(timestamp_dirs), 1,
                "all backups from one restore run must share a single timestamp directory"
            )
            ts_dir = timestamp_dirs[0]
            self.assertTrue((ts_dir / ".zshrc").exists())
            self.assertTrue((ts_dir / ".bashrc").exists())


if __name__ == "__main__":
    unittest.main()
