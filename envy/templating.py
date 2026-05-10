import re
from envy.secret_managers import SECRET_MANAGER_PROVIDERS, SecretManagerError

def parse_template_token(value):
    """
    Recognize and extract supported secret manager template tokens.
    Example: {{ doppler.secret("DB_PASSWORD") }}
    Returns (provider, method, args) or None if not a token.
    """
    pattern = r"\{\{\s*(\w+)\.(\w+)\((.*?)\)\s*\}\}"
    match = re.fullmatch(pattern, value.strip())
    if not match:
        return None
    provider, method, args_str = match.groups()
    # Only support doppler.secret("NAME") for now
    args = []
    for arg in re.findall(r'"([^"]+)"|\'([^']+)\'', args_str):
        args.append(arg[0] or arg[1])
    return provider, method, args

def interpolate_env_value(value):
    token = parse_template_token(value)
    if not token:
        return value  # Not a template token
    provider, method, args = token
    if provider not in SECRET_MANAGER_PROVIDERS:
        raise SecretManagerError(f"Unsupported secret manager provider: {provider}")
    if method != 'secret':
        raise SecretManagerError(f"Unsupported method '{method}' for provider '{provider}'")
    manager = SECRET_MANAGER_PROVIDERS[provider]()
    try:
        secret = manager.fetch_secret(*args)
        return secret
    except SecretManagerError as e:
        raise SecretManagerError(f"Error fetching secret for token '{value}': {e}")
