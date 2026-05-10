/**
 * Unit tests for the template parser and resolver.
 *
 * Secret provider calls are mocked so these tests run without
 * the op or doppler CLIs installed.
 */

import { parseArgs, parseTokens, resolveTemplate } from './template';

// ---------------------------------------------------------------------------
// Mock the provider registry so no real CLI calls are made
// ---------------------------------------------------------------------------

jest.mock('./providers/index', () => {
  const mockOpProvider = {
    fetch: jest.fn(async (method: string, args: string[]) => {
      if (method === 'item' && args[0] === 'Dev' && args[1] === 'api_key') {
        return 'mock-op-secret-value';
      }
      if (method === 'read' && args[0] === 'op://Dev/api_key/password') {
        return 'mock-op-read-value';
      }
      throw new Error(`op: secret not found: ${args.join(', ')}`);
    }),
  };

  const mockDopplerProvider = {
    fetch: jest.fn(async (method: string, args: string[]) => {
      if (method === 'secret' && args[0] === 'DB_PASSWORD') {
        return 'mock-doppler-secret';
      }
      throw new Error(`doppler: secret not found: ${args[0]}`);
    }),
  };

  return {
    getProvider: (name: string) => {
      if (name === 'op') return mockOpProvider;
      if (name === 'doppler') return mockDopplerProvider;
      return undefined;
    },
    listProviders: () => ['op', 'doppler'],
  };
});

// ---------------------------------------------------------------------------
// parseArgs
// ---------------------------------------------------------------------------

describe('parseArgs', () => {
  test('parses double-quoted args', () => {
    expect(parseArgs('"Vault", "api_key"')).toEqual(['Vault', 'api_key']);
  });

  test('parses single-quoted args', () => {
    expect(parseArgs("'DB_PASSWORD'")).toEqual(['DB_PASSWORD']);
  });

  test('parses mixed quotes', () => {
    expect(parseArgs('"Vault", \'item\', "field"')).toEqual(['Vault', 'item', 'field']);
  });

  test('handles escaped quotes inside strings', () => {
    expect(parseArgs('"it\'s a secret"')).toEqual(["it's a secret"]);
  });

  test('returns empty array for empty input', () => {
    expect(parseArgs('')).toEqual([]);
    expect(parseArgs('   ')).toEqual([]);
  });

  test('parses single unquoted arg', () => {
    expect(parseArgs('MY_SECRET')).toEqual(['MY_SECRET']);
  });
});

// ---------------------------------------------------------------------------
// parseTokens
// ---------------------------------------------------------------------------

describe('parseTokens', () => {
  test('parses a plain token', () => {
    const tokens = parseTokens('{{ MY_VAR }}');
    expect(tokens).toHaveLength(1);
    expect(tokens[0]).toMatchObject({ type: 'plain', name: 'MY_VAR' });
  });

  test('parses an op.item secret token', () => {
    const tokens = parseTokens('{{ op.item("Dev", "api_key") }}');
    expect(tokens).toHaveLength(1);
    expect(tokens[0]).toMatchObject({
      type: 'secret',
      provider: 'op',
      method: 'item',
      args: ['Dev', 'api_key'],
    });
  });

  test('parses a doppler.secret token', () => {
    const tokens = parseTokens('{{ doppler.secret("DB_PASSWORD") }}');
    expect(tokens).toHaveLength(1);
    expect(tokens[0]).toMatchObject({
      type: 'secret',
      provider: 'doppler',
      method: 'secret',
      args: ['DB_PASSWORD'],
    });
  });

  test('treats unknown provider prefix as plain token', () => {
    const tokens = parseTokens('{{ unknown.method("arg") }}');
    expect(tokens).toHaveLength(1);
    expect(tokens[0].type).toBe('plain');
  });

  test('parses mixed tokens', () => {
    const tokens = parseTokens('{{ HOST }}\n{{ op.item("Dev", "api_key") }}\n{{ PORT }}');
    expect(tokens).toHaveLength(3);
    expect(tokens[0]).toMatchObject({ type: 'plain', name: 'HOST' });
    expect(tokens[1]).toMatchObject({ type: 'secret', provider: 'op' });
    expect(tokens[2]).toMatchObject({ type: 'plain', name: 'PORT' });
  });
});

// ---------------------------------------------------------------------------
// resolveTemplate
// ---------------------------------------------------------------------------

describe('resolveTemplate', () => {
  test('resolves plain tokens from env', async () => {
    const result = await resolveTemplate('HOST={{ HOST }}', { env: { HOST: 'localhost' } });
    expect(result).toBe('HOST=localhost');
  });

  test('leaves unresolved plain tokens as-is (non-strict)', async () => {
    const result = await resolveTemplate('{{ MISSING }}', { env: {} });
    expect(result).toBe('{{ MISSING }}');
  });

  test('throws in strict mode for missing plain token', async () => {
    await expect(resolveTemplate('{{ MISSING }}', { env: {}, strict: true })).rejects.toThrow(
      'MISSING'
    );
  });

  test('resolves op.item secret token', async () => {
    const result = await resolveTemplate('KEY={{ op.item("Dev", "api_key") }}');
    expect(result).toBe('KEY=mock-op-secret-value');
  });

  test('resolves doppler.secret token', async () => {
    const result = await resolveTemplate('DB_PASSWORD={{ doppler.secret("DB_PASSWORD") }}');
    expect(result).toBe('DB_PASSWORD=mock-doppler-secret');
  });

  test('resolves mixed plain and secret tokens', async () => {
    const template = 'HOST={{ HOST }}\nAPI_KEY={{ op.item("Dev", "api_key") }}';
    const result = await resolveTemplate(template, { env: { HOST: 'localhost' } });
    expect(result).toBe('HOST=localhost\nAPI_KEY=mock-op-secret-value');
  });

  test('throws with clear message when secret fetch fails', async () => {
    await expect(
      resolveTemplate('KEY={{ op.item("Dev", "nonexistent") }}')
    ).rejects.toThrow('Failed to resolve secret token');
  });

  test('does not resolve tokens with unknown provider prefix', async () => {
    const result = await resolveTemplate('{{ unknown.method("arg") }}', { env: {} });
    // Treated as plain token with no env match — left as-is
    expect(result).toBe('{{ unknown.method("arg") }}');
  });
});
