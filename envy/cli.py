import sys
import os
from envy.templating import interpolate_env_value, SecretManagerError

HELP_TEXT = """
Usage: envy restore [OPTIONS]

Restores environment variables from .env file, supporting template tokens for external Secret Managers.

New Feature: You can use template tokens in your .env file to fetch secrets dynamically at restore time.
Example:
  API_KEY={{ doppler.secret("MY_API_KEY") }}

Supported providers: doppler

Options:
  --help    Show this message and exit.
"""

def restore_env():
    env_path = '.env'
    if not os.path.exists(env_path):
        print(".env file not found.", file=sys.stderr)
        sys.exit(1)
    with open(env_path) as f:
        lines = f.readlines()
    restored = {}
    for line in lines:
        line = line.strip()
        if not line or line.startswith('#'):
            continue
        if '=' not in line:
            continue
        key, value = line.split('=', 1)
        value = value.strip()
        try:
            value = interpolate_env_value(value)
        except SecretManagerError as e:
            print(f"Error: {e}", file=sys.stderr)
            sys.exit(2)
        restored[key] = value
        os.environ[key] = value
    print("Environment restored.")

def main(argv=None):
    argv = argv or sys.argv[1:]
    if '--help' in argv or '-h' in argv:
        print(HELP_TEXT)
        return 0
    if argv and argv[0] == 'restore':
        restore_env()
        return 0
    print(HELP_TEXT)
    return 1
