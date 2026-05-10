/**
 * .envyrc loader and type definitions.
 *
 * .envyrc is an optional JSON file at the project root that configures
 * per-project defaults for secret providers and other envy settings.
 *
 * Example .envyrc:
 * {
 *   "doppler": {
 *     "project": "my-app",
 *     "config": "prd"
 *   }
 * }
 */

import * as fs from 'fs';
import * as path from 'path';

export interface DopplerConfig {
  /** Doppler project name (overrides Doppler's own scoped config). */
  project?: string;
  /** Doppler config/environment name (e.g. "prd", "dev"). */
  config?: string;
}

export interface EnvyRc {
  /** Doppler provider defaults. */
  doppler?: DopplerConfig;
}

let _cache: EnvyRc | null | undefined = undefined;

/**
 * Load and parse .envyrc from the current working directory.
 * Returns null if the file does not exist.
 * Throws a descriptive error if the file exists but is invalid JSON.
 * Results are cached for the lifetime of the process.
 */
export async function loadEnvyRc(cwd: string = process.cwd()): Promise<EnvyRc | null> {
  if (_cache !== undefined) {
    return _cache;
  }

  const rcPath = path.join(cwd, '.envyrc');

  if (!fs.existsSync(rcPath)) {
    _cache = null;
    return null;
  }

  try {
    const raw = fs.readFileSync(rcPath, 'utf8');
    _cache = JSON.parse(raw) as EnvyRc;
    return _cache;
  } catch (err) {
    throw new Error(
      `.envyrc found at ${rcPath} but could not be parsed as JSON: ${(err as Error).message}`
    );
  }
}

/** Reset the cache (useful in tests). */
export function resetEnvyRcCache(): void {
  _cache = undefined;
}
