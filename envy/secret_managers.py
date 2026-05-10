import os
import requests

class SecretManagerError(Exception):
    pass

class SecretManagerProvider:
    def fetch_secret(self, *args, **kwargs):
        raise NotImplementedError

class DopplerSecretManager(SecretManagerProvider):
    """
    Fetches secrets from Doppler using the Doppler CLI token or API token.
    """
    def __init__(self, token=None):
        self.token = token or os.environ.get("DOPPLER_TOKEN")
        if not self.token:
            raise SecretManagerError("Doppler token not found. Set DOPPLER_TOKEN environment variable.")

    def fetch_secret(self, secret_name):
        # Doppler CLI/API endpoint for secrets
        url = f"https://api.doppler.com/v3/configs/config/secrets/download?format=json"
        headers = {"Authorization": f"Bearer {self.token}"}
        try:
            resp = requests.get(url, headers=headers)
            if resp.status_code != 200:
                raise SecretManagerError(f"Doppler API error: {resp.status_code} {resp.text}")
            secrets = resp.json().get("data", {})
            if secret_name not in secrets:
                raise SecretManagerError(f"Secret '{secret_name}' not found in Doppler response.")
            return secrets[secret_name]
        except Exception as e:
            raise SecretManagerError(f"Failed to fetch Doppler secret '{secret_name}': {e}")

SECRET_MANAGER_PROVIDERS = {
    'doppler': DopplerSecretManager,
    # Future: 'op': OnePasswordSecretManager, etc.
}
