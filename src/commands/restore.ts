/**
 * envy restore — resolves a .env template and writes the output .env file.
 *
 * Token syntax supported in templates
 * ─────────────────────────────────────
 * Plain variable substitution:
 *   {{ VARIABLE_NAME }}
 *   Replaced with the value of the environment variable VARIABLE_NAME.
 *
 * 1Password (op) secrets:
 *   {{ op.item("VaultName", "ItemTitle") }}
 *   {{ op.item("VaultName", "ItemTitle", "FieldLabel") }}
 *   {{ op.read("op://VaultName/ItemTitle/FieldLabel") }}
 *   Requires the `op` CLI (>= v2) to be installed and authenticated.
 *   Install: https://developer.1password.com/docs/cli/get-started/
 *
 * Doppler secrets:
 *   {{ doppler.secret("SECRET_NAME") }}
 *   {{ doppler.secret("SECRET_NAME", "project", "config") }}
 *   Requires the `doppler` CLI to be installed and authenticated.
 *   Install: https://docs.doppler.com/docs/install-cli
 *   Project/config defaults can be set in .envyrc.
 *
 * Usage:
 *   envy restore [--template <path>] [--output <path>] [--strict]
 *
 * Options:
 *   --template, -t   Path to the .env template file  (default: .env.template)
 *   --output,   -o   Path to write the resolved .env  (default: .env)
 *   --strict         Fail if any plain {{ VAR }} token has no matching env var
 *   --help,     -h   Show this help message
 */

import * as fs from 'fs';
import * as path from 'path';
import { resolveTemplate } from '../template';

// ---------------------------------------------------------------------------
// Argument parsing (minimal, no external dependency)
// ---------------------------------------------------------------------------

interface RestoreOptions {
  template: string;
  output: string;
  strict: boolean;
  help: boolean;
}

function parseArgs(argv: string[]): RestoreOptions {
  const opts: RestoreOptions = {
    template: '.env.template',
    output: '.env',
    strict: false,
    help: false,
  };

  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === '--help' || arg === '-h') {
      opts.help = true;
    } else if ((arg === '--template' || arg === '-t') && argv[i + 1]) {
      opts.template = argv[++i];
    } else if ((arg === '--output' || arg === '-o') && argv[i + 1]) {
      opts.output = argv[++i];
    } else if (arg === '--strict') {
      opts.strict = true;
    }
  }

  return opts;
}

// ---------------------------------------------------------------------------
// Help text
// ---------------------------------------------------------------------------

const HELP = `
envy restore — resolve a .env template and write the output .env file

USAGE
  envy restore [options]

OPTIONS
  -t, --template <path>   Path to the .env template file  (default: .env.template)
  -o, --output   <path>   Path to write the resolved .env  (default: .env)
      --strict            Fail if any plain {{ VAR }} token has no matching env var
  -h, --help              Show this help message

TOKEN SYNTAX
  Plain variable substitution:
    {{ VARIABLE_NAME }}
    Replaced with the value of the environment variable VARIABLE_NAME.

  1Password (op) secrets  [requires op CLI >= v2, authenticated]:
    {{ op.item("VaultName", "ItemTitle") }}
    {{ op.item("VaultName", "ItemTitle", "FieldLabel") }}
    {{ op.read("op://VaultName/ItemTitle/FieldLabel") }}
    Install: https://developer.1password.com/docs/cli/get-started/

  Doppler secrets  [requires doppler CLI, authenticated]:
    {{ doppler.secret("SECRET_NAME") }}
    {{ doppler.secret("SECRET_NAME", "project", "config") }}
    Install: https://docs.doppler.com/docs/install-cli
    Project/config defaults can be set in .envyrc:
      { "doppler": { "project": "my-app", "config": "prd" } }

EXAMPLES
  # Resolve .env.template -> .env using env vars and 1Password
  envy restore

  # Use a custom template path
  envy restore --template config/.env.tpl --output config/.env

  # Fail on unresolved plain variables
  envy restore --strict
`.trim();

// ---------------------------------------------------------------------------
// Main restore handler
// ---------------------------------------------------------------------------

export async function restoreCommand(argv: string[] = process.argv.slice(2)): Promise<void> {
  const opts = parseArgs(argv);

  if (opts.help) {
    console.log(HELP);
    return;
  }

  const templatePath = path.resolve(opts.template);
  const outputPath = path.resolve(opts.output);

  // Read template
  if (!fs.existsSync(templatePath)) {
    console.error(`envy restore: template file not found: ${templatePath}`);
    process.exit(1);
  }

  const templateContent = fs.readFileSync(templatePath, 'utf8');

  // Resolve all tokens (plain + secret).
  // resolveTemplate throws with a human-readable message on any failure.
  // We catch here to ensure we never write a partial .env.
  let resolved: string;
  try {
    resolved = await resolveTemplate(templateContent, { strict: opts.strict });
  } catch (err) {
    console.error(`envy restore: ${(err as Error).message}`);
    process.exit(1);
  }

  // Write output atomically: write to a temp file then rename,
  // so a failure mid-write never leaves a corrupt .env.
  const tmpPath = `${outputPath}.envy.tmp`;
  try {
    fs.writeFileSync(tmpPath, resolved, { encoding: 'utf8', mode: 0o600 });
    fs.renameSync(tmpPath, outputPath);
  } catch (err) {
    // Clean up temp file if rename failed
    if (fs.existsSync(tmpPath)) {
      try { fs.unlinkSync(tmpPath); } catch { /* ignore */ }
    }
    console.error(`envy restore: failed to write output file ${outputPath}: ${(err as Error).message}`);
    process.exit(1);
  }

  console.log(`envy restore: wrote ${outputPath}`);
}

// Allow direct execution: ts-node src/commands/restore.ts [args]
if (require.main === module) {
  restoreCommand().catch((err) => {
    console.error(err);
    process.exit(1);
  });
}
