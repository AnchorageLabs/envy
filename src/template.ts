/**
 * Template parser and resolver for envy .env templates.
 *
 * Supported token syntaxes
 * ────────────────────────
 * Plain variable substitution (existing behaviour, unchanged):
 *   {{ VARIABLE_NAME }}
 *
 * Secret provider tokens (new):
 *   {{ op.item("VaultName", "ItemTitle") }}
 *   {{ op.item("VaultName", "ItemTitle", "FieldLabel") }}
 *   {{ op.read("op://VaultName/ItemTitle/FieldLabel") }}
 *   {{ doppler.secret("SECRET_NAME") }}
 *   {{ doppler.secret("SECRET_NAME", "project", "config") }}
 *
 * Unknown prefixes and plain tokens that have no matching environment variable
 * are left as-is (same behaviour as before) unless strict mode is enabled.
 */

import { getProvider } from './providers/index';

// ---------------------------------------------------------------------------
// Token types
// ---------------------------------------------------------------------------

export interface PlainToken {
  type: 'plain';
  raw: string;       // full {{ ... }} match
  name: string;      // trimmed inner content
}

export interface SecretToken {
  type: 'secret';
  raw: string;       // full {{ ... }} match
  provider: string;  // e.g. "op", "doppler"
  method: string;    // e.g. "item", "secret", "read"
  args: string[];    // parsed string arguments
}

export type Token = PlainToken | SecretToken;

// ---------------------------------------------------------------------------
// Regex patterns
// ---------------------------------------------------------------------------

/**
 * Matches any {{ ... }} block.
 * Group 1: the inner content (trimmed by the parser).
 */
const TOKEN_RE = /\{\{([^}]+)\}\}/g;

/**
 * Matches a secret provider call inside a token:
 *   <provider>.<method>(<args>)
 * Group 1: provider name
 * Group 2: method name
 * Group 3: raw argument string (everything inside the outer parentheses)
 */
const SECRET_TOKEN_RE = /^([A-Za-z][A-Za-z0-9_]*)\s*\.\s*([A-Za-z][A-Za-z0-9_]*)\s*\((.*)\)\s*$/s;

// ---------------------------------------------------------------------------
// Argument parser
// ---------------------------------------------------------------------------

/**
 * Parse a comma-separated list of quoted string arguments.
 * Supports single and double quotes. Ignores unquoted whitespace between args.
 * Returns an array of unescaped string values.
 *
 * Examples:
 *   '"Vault", "api_key"'  -> ["Vault", "api_key"]
 *   "'DB_PASSWORD'"       -> ["DB_PASSWORD"]
 */
export function parseArgs(raw: string): string[] {
  const args: string[] = [];
  let i = 0;
  const s = raw.trim();

  while (i < s.length) {
    // Skip whitespace and commas between arguments
    if (s[i] === ',' || s[i] === ' ' || s[i] === '\t' || s[i] === '\n' || s[i] === '\r') {
      i++;
      continue;
    }

    // Quoted string
    if (s[i] === '"' || s[i] === "'") {
      const quote = s[i];
      i++;
      let value = '';
      while (i < s.length && s[i] !== quote) {
        if (s[i] === '\\' && i + 1 < s.length) {
          i++; // skip backslash
          value += s[i];
        } else {
          value += s[i];
        }
        i++;
      }
      i++; // skip closing quote
      args.push(value);
      continue;
    }

    // Unquoted token (bare word) — consume until comma or end
    let value = '';
    while (i < s.length && s[i] !== ',') {
      value += s[i];
      i++;
    }
    const trimmed = value.trim();
    if (trimmed.length > 0) {
      args.push(trimmed);
    }
  }

  return args;
}

// ---------------------------------------------------------------------------
// Token parser
// ---------------------------------------------------------------------------

/**
 * Parse all {{ }} tokens in a template string.
 * Returns an array of Token objects in order of appearance.
 */
export function parseTokens(template: string): Token[] {
  const tokens: Token[] = [];
  let match: RegExpExecArray | null;
  TOKEN_RE.lastIndex = 0;

  while ((match = TOKEN_RE.exec(template)) !== null) {
    const raw = match[0];
    const inner = match[1].trim();

    const secretMatch = SECRET_TOKEN_RE.exec(inner);
    if (secretMatch) {
      const [, provider, method, rawArgs] = secretMatch;
      // Only treat as a secret token if the provider is registered.
      // Unknown provider prefixes fall through to plain token handling
      // to preserve backward compatibility.
      if (getProvider(provider) !== undefined) {
        tokens.push({
          type: 'secret',
          raw,
          provider,
          method,
          args: parseArgs(rawArgs),
        });
        continue;
      }
    }

    tokens.push({
      type: 'plain',
      raw,
      name: inner,
    });
  }

  return tokens;
}

// ---------------------------------------------------------------------------
// Template resolver
// ---------------------------------------------------------------------------

export interface ResolveOptions {
  /**
   * Environment variables used to resolve plain {{ VAR }} tokens.
   * Defaults to process.env.
   */
  env?: Record<string, string | undefined>;

  /**
   * When true, a plain token with no matching env var throws an error
   * instead of being left as-is.
   */
  strict?: boolean;
}

/**
 * Resolve all tokens in a template string.
 *
 * - Plain tokens are substituted from `options.env` (defaults to process.env).
 * - Secret tokens are resolved by calling the appropriate provider.
 * - If a provider CLI is missing or a secret fetch fails, an error is thrown
 *   with a human-readable message; no partial output is produced.
 *
 * @param template  Raw template string.
 * @param options   Resolution options.
 * @returns         Fully resolved string.
 */
export async function resolveTemplate(
  template: string,
  options: ResolveOptions = {}
): Promise<string> {
  const env = options.env ?? (process.env as Record<string, string | undefined>);
  const strict = options.strict ?? false;

  const tokens = parseTokens(template);

  // Collect all secret resolutions in parallel for performance,
  // but abort entirely if any single one fails.
  const secretResolutions = new Map<string, Promise<string>>();

  for (const token of tokens) {
    if (token.type === 'secret') {
      const key = token.raw; // raw is unique per occurrence; use index if duplicates matter
      if (!secretResolutions.has(key)) {
        const provider = getProvider(token.provider)!;
        secretResolutions.set(
          key,
          provider.fetch(token.method, token.args).catch((err: Error) => {
            // Re-throw with context about which token caused the failure
            throw new Error(
              `Failed to resolve secret token ${token.raw}: ${err.message}`
            );
          })
        );
      }
    }
  }

  // Await all secret fetches; if any throws, the whole resolve fails
  // (no partial .env is written — the caller is responsible for that guarantee).
  const resolvedSecrets = new Map<string, string>();
  for (const [key, promise] of secretResolutions) {
    resolvedSecrets.set(key, await promise);
  }

  // Perform substitutions in a single pass
  let result = template;
  for (const token of tokens) {
    if (token.type === 'secret') {
      const value = resolvedSecrets.get(token.raw)!;
      // Replace only the first occurrence matching this raw token to handle
      // duplicate tokens correctly (each was resolved independently above).
      result = result.replace(token.raw, value);
    } else {
      // Plain token
      const value = env[token.name];
      if (value !== undefined) {
        result = result.replace(token.raw, value);
      } else if (strict) {
        throw new Error(
          `Template variable "${token.name}" is not set in the environment.`
        );
      }
      // else: leave the token as-is (existing behaviour)
    }
  }

  return result;
}
