/**
 * 1Password CLI (op) secret provider.
 *
 * Minimum required CLI version: op 2.x
 * Install: https://developer.1password.com/docs/cli/get-started/
 *
 * Supported template methods:
 *   {{ op.item("VaultName", "ItemTitle", "FieldLabel") }}
 *     -> shells out to: op item get "ItemTitle" --vault "VaultName" --fields label="FieldLabel" --reveal
 *
 *   {{ op.read("op://VaultName/ItemTitle/FieldLabel") }}
 *     -> shells out to: op read "op://VaultName/ItemTitle/FieldLabel"
 */

import { execFile } from 'child_process';
import { promisify } from 'util';
import type { SecretProvider } from './index';

const execFileAsync = promisify(execFile);

export class OpProvider implements SecretProvider {
  async fetch(method: string, args: string[]): Promise<string> {
    switch (method) {
      case 'item':
        return this.fetchItem(args);
      case 'read':
        return this.fetchRead(args);
      default:
        throw new Error(
          `1Password provider: unknown method "${method}". Supported methods: item, read.`
        );
    }
  }

  /**
   * op.item("VaultName", "ItemTitle")           -> first password field
   * op.item("VaultName", "ItemTitle", "field")  -> specific field label
   */
  private async fetchItem(args: string[]): Promise<string> {
    if (args.length < 2) {
      throw new Error(
        '1Password op.item() requires at least 2 arguments: op.item("VaultName", "ItemTitle") or op.item("VaultName", "ItemTitle", "FieldLabel")'
      );
    }
    const [vault, item, field] = args;
    const cliArgs = ['item', 'get', item, '--vault', vault, '--reveal'];
    if (field) {
      cliArgs.push('--fields', `label=${field}`);
    } else {
      cliArgs.push('--fields', 'label=password');
    }
    return this.run(cliArgs);
  }

  /**
   * op.read("op://VaultName/ItemTitle/FieldLabel")
   */
  private async fetchRead(args: string[]): Promise<string> {
    if (args.length < 1) {
      throw new Error(
        '1Password op.read() requires 1 argument: op.read("op://VaultName/ItemTitle/FieldLabel")'
      );
    }
    return this.run(['read', args[0]]);
  }

  private async run(args: string[]): Promise<string> {
    try {
      const { stdout } = await execFileAsync('op', args, { env: process.env });
      return stdout.trim();
    } catch (err: unknown) {
      const error = err as NodeJS.ErrnoException & { stderr?: string };
      if (error.code === 'ENOENT') {
        throw new Error(
          '1Password CLI (op) not found.\n' +
            'Install it from https://developer.1password.com/docs/cli/get-started/ and sign in before running envy restore.'
        );
      }
      const detail = error.stderr?.trim() || error.message;
      throw new Error(`1Password CLI error: ${detail}`);
    }
  }
}
