/**
 * Provider registry for secret manager integrations.
 *
 * To add a new provider:
 *   1. Create src/providers/<name>.ts implementing SecretProvider.
 *   2. Import it here and add it to the PROVIDERS map.
 *   No changes to the core restore pipeline are required.
 */

import { OpProvider } from './op';
import { DopplerProvider } from './doppler';

/** Contract every secret provider must satisfy. */
export interface SecretProvider {
  /**
   * Fetch a secret value.
   * @param method  The method name used in the template token (e.g. "item", "secret").
   * @param args    Positional string arguments parsed from the token.
   * @returns       Resolved plaintext secret value.
   * @throws        A human-readable Error if the CLI is missing or the secret cannot be found.
   */
  fetch(method: string, args: string[]): Promise<string>;
}

/** Registry mapping provider prefix names to their implementations. */
const PROVIDERS: Record<string, SecretProvider> = {
  op: new OpProvider(),
  doppler: new DopplerProvider(),
};

/**
 * Look up a registered provider by name.
 * Returns undefined when the name is not a known secret provider
 * (so the caller can treat the token as a plain variable).
 */
export function getProvider(name: string): SecretProvider | undefined {
  return PROVIDERS[name];
}

/** List all registered provider prefix names. */
export function listProviders(): string[] {
  return Object.keys(PROVIDERS);
}
