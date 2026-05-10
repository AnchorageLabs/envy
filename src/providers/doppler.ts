/**
 * Doppler CLI secret provider.
 *
 * Install: https://docs.doppler.com/docs/install-cli
 *
 * Supported template methods:
 *   {{ doppler.secret("SECRET_NAME") }}
 *     -> shells out to: doppler secrets get SECRET_NAME --plain
 *        (uses the project/config from the local Doppler scope or .envyrc)
 *
 *   {{ doppler.secret("SECRET_NAME", "my-project", "prd") }}
 *     -> doppler secrets get SECRET_NAME --project my-project --config prd --plain
 *
 * Project/config defaults can be set in .envyrc:
 *   { "doppler": { "project": "my-project", "config": "prd" } }
 */

import { execFile } from 'child_process';
import { promisify } from 'util';
import { loadEnvyRc } from '../config';
import type { SecretProvider } from './index';

const execFileAsync = promisify(execFile);

export class DopplerProvider implements SecretProvider {
  async fetch(method: string, args: string[]): Promise<string> {
    switch (method) {
      case 'secret':
        return this.fetchSecret(args);
      default:
        throw new Error(
          `Doppler provider: unknown method "${method}". Supported methods: secret.`
        );
    }
  }

  private async fetchSecret(args: string[]): Promise<string> {
    if (args.length < 1) {
      throw new Error(
        'Doppler doppler.secret() requires at least 1 argument: doppler.secret("SECRET_NAME")'
      );
    }

    const [secretName, inlineProject, inlineConfig] = args;

    // Resolve project/config: inline args > .envyrc > Doppler's own scoped config
    const rc = await loadEnvyRc();
    const project = inlineProject || rc?.doppler?.project;
    const config = inlineConfig || rc?.doppler?.config;

    const cliArgs = ['secrets', 'get', secretName, '--plain'];
    if (project) {
      cliArgs.push('--project', project);
    }
    if (config) {
      cliArgs.push('--config', config);
    }

    return this.run(cliArgs);
  }

  private async run(args: string[]): Promise<string> {
    try {
      const { stdout } = await execFileAsync('doppler', args, { env: process.env });
      return stdout.trim();
    } catch (err: unknown) {
      const error = err as NodeJS.ErrnoException & { stderr?: string };
      if (error.code === 'ENOENT') {
        throw new Error(
          'Doppler CLI (doppler) not found.\n' +
            'Install it from https://docs.doppler.com/docs/install-cli and run `doppler login` before running envy restore.'
        );
      }
      const detail = error.stderr?.trim() || error.message;
      throw new Error(`Doppler CLI error: ${detail}`);
    }
  }
}
