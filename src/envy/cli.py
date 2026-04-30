from __future__ import annotations

import argparse
import difflib
import os
import platform
import re
import shutil
import socket
import subprocess
import sys
import tomllib
from dataclasses import dataclass
from pathlib import Path


DEFAULT_CONFIG = """# Envy tracks dotfiles from your home directory.
# Paths are relative to $HOME unless absolute.
files = [
  ".zshrc",
  ".gitconfig",
]

[machine]
# Add values used by templates, for example:
# work_email = "you@company.com"
"""


TOKEN_RE = re.compile(r"{{\s*([a-zA-Z0-9_.-]+)\s*}}")


@dataclass(frozen=True)
class Settings:
    repo: Path
    home: Path
    config_path: Path
    files: list[str]
    machine: dict[str, str]


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    try:
        return args.func(args)
    except EnvyError as exc:
        print(f"envy: {exc}", file=sys.stderr)
        return 1


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="envy",
        description="Backup, restore, sync, and bootstrap dotfiles across machines.",
    )
    parser.add_argument(
        "--repo",
        default=os.environ.get("ENVY_REPO", "."),
        help="dotfiles repo path, defaults to ENVY_REPO or current directory",
    )
    parser.add_argument(
        "--home",
        default=os.environ.get("ENVY_HOME", str(Path.home())),
        help="home directory to read/write dotfiles from",
    )

    sub = parser.add_subparsers(dest="command", required=True)

    init = sub.add_parser("init", help="create .envy/config.toml and repo folders")
    init.set_defaults(func=cmd_init)

    backup = sub.add_parser("backup", help="copy configured dotfiles into this repo")
    backup.add_argument("--file", action="append", dest="files", help="extra file to back up")
    backup.set_defaults(func=cmd_backup)

    restore = sub.add_parser("restore", help="restore configured dotfiles into $HOME")
    restore.add_argument("--dry-run", action="store_true", help="show changes without writing")
    restore.add_argument("--force", action="store_true", help="overwrite existing files")
    restore.set_defaults(func=cmd_restore)

    status = sub.add_parser("status", help="show tracked files and pending diffs")
    status.set_defaults(func=cmd_status)

    sync = sub.add_parser("sync", help="commit and optionally pull/push with git")
    sync.add_argument("-m", "--message", default="sync dotfiles", help="commit message")
    sync.add_argument("--pull", action="store_true", help="run git pull --rebase before commit")
    sync.add_argument("--push", action="store_true", help="run git push after commit")
    sync.set_defaults(func=cmd_sync)

    bootstrap = sub.add_parser("bootstrap", help="restore dotfiles, then run scripts/bootstrap if present")
    bootstrap.add_argument("--dry-run", action="store_true", help="show restore changes without writing")
    bootstrap.add_argument("--force", action="store_true", help="overwrite existing files")
    bootstrap.set_defaults(func=cmd_bootstrap)

    return parser


def cmd_init(args: argparse.Namespace) -> int:
    repo = Path(args.repo).expanduser().resolve()
    (repo / ".envy").mkdir(parents=True, exist_ok=True)
    (repo / "home").mkdir(exist_ok=True)
    (repo / "templates").mkdir(exist_ok=True)
    (repo / "scripts").mkdir(exist_ok=True)

    config_path = repo / ".envy" / "config.toml"
    if not config_path.exists():
        config_path.write_text(DEFAULT_CONFIG, encoding="utf-8")
        print(f"created {config_path}")
    else:
        print(f"exists {config_path}")

    if not (repo / ".git").exists():
        run(["git", "init"], cwd=repo)
    return 0


def cmd_backup(args: argparse.Namespace) -> int:
    settings = load_settings(args)
    files = merge_files(settings.files, args.files or [])
    for name in files:
        source = resolve_home_path(settings.home, name)
        target = repo_home_path(settings.repo, settings.home, source)
        if not source.exists():
            print(f"skip missing {source}")
            continue
        copy_path(source, target)
        print(f"backed up {source} -> {target}")
    return 0


def cmd_restore(args: argparse.Namespace) -> int:
    settings = load_settings(args)
    restore(settings, dry_run=args.dry_run, force=args.force)
    return 0


def cmd_status(args: argparse.Namespace) -> int:
    settings = load_settings(args)
    dirty = False
    for name in settings.files:
        source = resolve_home_path(settings.home, name)
        stored = stored_path(settings.repo, name)
        template = template_path(settings.repo, name)

        if template.exists():
            rendered = render_template(template.read_text(encoding="utf-8"), settings)
            current = source.read_text(encoding="utf-8") if source.exists() and source.is_file() else ""
            if rendered != current:
                dirty = True
                print_diff(source, current, rendered)
            else:
                print(f"ok {name}")
        elif stored.exists() and stored.is_file():
            current = source.read_text(encoding="utf-8") if source.exists() and source.is_file() else ""
            stored_text = stored.read_text(encoding="utf-8")
            if stored_text != current:
                dirty = True
                print_diff(source, current, stored_text)
            else:
                print(f"ok {name}")
        elif stored.exists():
            print(f"tracked directory {name}")
        else:
            dirty = True
            print(f"missing in repo {name}")
    return 1 if dirty else 0


def cmd_sync(args: argparse.Namespace) -> int:
    settings = load_settings(args)
    ensure_git_repo(settings.repo)
    if args.pull:
        run(["git", "pull", "--rebase"], cwd=settings.repo)
    run(["git", "add", "."], cwd=settings.repo)
    diff = subprocess.run(
        ["git", "diff", "--cached", "--quiet"],
        cwd=settings.repo,
        check=False,
    )
    if diff.returncode == 0:
        print("nothing to commit")
    else:
        run(["git", "commit", "-m", args.message], cwd=settings.repo)
    if args.push:
        run(["git", "push"], cwd=settings.repo)
    return 0


def cmd_bootstrap(args: argparse.Namespace) -> int:
    settings = load_settings(args)
    restore(settings, dry_run=args.dry_run, force=args.force)
    script = settings.repo / "scripts" / "bootstrap"
    if script.exists() and not args.dry_run:
        run([str(script)], cwd=settings.repo)
    elif script.exists():
        print(f"would run {script}")
    return 0


def load_settings(args: argparse.Namespace) -> Settings:
    repo = Path(args.repo).expanduser().resolve()
    home = Path(args.home).expanduser().resolve()
    config_path = repo / ".envy" / "config.toml"
    if not config_path.exists():
        raise EnvyError(f"missing {config_path}; run `envy init` first")
    data = tomllib.loads(config_path.read_text(encoding="utf-8"))
    files = data.get("files", [])
    if not isinstance(files, list) or not all(isinstance(item, str) for item in files):
        raise EnvyError("config key `files` must be a list of strings")
    machine = data.get("machine", {})
    if not isinstance(machine, dict):
        raise EnvyError("config key `machine` must be a table")
    return Settings(repo=repo, home=home, config_path=config_path, files=files, machine=machine)


def restore(settings: Settings, *, dry_run: bool, force: bool) -> None:
    for name in settings.files:
        target = resolve_home_path(settings.home, name)
        source = template_path(settings.repo, name)
        rendered: str | None = None

        if source.exists():
            rendered = render_template(source.read_text(encoding="utf-8"), settings)
        else:
            source = stored_path(settings.repo, name)
            if not source.exists():
                print(f"skip missing {name}")
                continue

        if target.exists() and not force and not dry_run:
            raise EnvyError(f"{target} exists; use --force to overwrite")

        if dry_run:
            print(f"would restore {source} -> {target}")
            continue

        target.parent.mkdir(parents=True, exist_ok=True)
        if rendered is not None:
            target.write_text(rendered, encoding="utf-8")
        elif source.is_dir():
            if target.exists():
                shutil.rmtree(target)
            shutil.copytree(source, target)
        else:
            shutil.copy2(source, target)
        print(f"restored {target}")


def render_template(text: str, settings: Settings) -> str:
    values = {
        "hostname": socket.gethostname(),
        "machine": socket.gethostname(),
        "os": platform.system().lower(),
        **{f"machine.{key}": str(value) for key, value in settings.machine.items()},
    }

    def replace(match: re.Match[str]) -> str:
        key = match.group(1)
        if key.startswith("env."):
            return os.environ.get(key.removeprefix("env."), "")
        return values.get(key, match.group(0))

    return TOKEN_RE.sub(replace, text)


def merge_files(configured: list[str], extra: list[str]) -> list[str]:
    seen: set[str] = set()
    files: list[str] = []
    for item in [*configured, *extra]:
        if item not in seen:
            files.append(item)
            seen.add(item)
    return files


def resolve_home_path(home: Path, name: str) -> Path:
    path = Path(name).expanduser()
    return path if path.is_absolute() else home / path


def repo_home_path(repo: Path, home: Path, source: Path) -> Path:
    try:
        relative = source.resolve().relative_to(home)
    except ValueError as exc:
        raise EnvyError(f"{source} is outside {home}") from exc
    return repo / "home" / relative


def stored_path(repo: Path, name: str) -> Path:
    path = Path(name)
    if path.is_absolute():
        path = Path(*path.parts[1:])
    return repo / "home" / path


def template_path(repo: Path, name: str) -> Path:
    path = Path(name)
    if path.is_absolute():
        path = Path(*path.parts[1:])
    return repo / "templates" / path


def copy_path(source: Path, target: Path) -> None:
    target.parent.mkdir(parents=True, exist_ok=True)
    if source.is_dir():
        if target.exists():
            shutil.rmtree(target)
        shutil.copytree(source, target)
    else:
        shutil.copy2(source, target)


def print_diff(path: Path, current: str, desired: str) -> None:
    diff = difflib.unified_diff(
        current.splitlines(keepends=True),
        desired.splitlines(keepends=True),
        fromfile=str(path),
        tofile="envy",
    )
    print("".join(diff), end="")


def ensure_git_repo(repo: Path) -> None:
    if not (repo / ".git").exists():
        raise EnvyError(f"{repo} is not a git repo; run `envy init` first")


def run(command: list[str], cwd: Path) -> None:
    subprocess.run(command, cwd=cwd, check=True)


class EnvyError(Exception):
    pass


if __name__ == "__main__":
    raise SystemExit(main())
