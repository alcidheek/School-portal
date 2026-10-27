import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';
import { login } from '../src/services/authService.ts';
import { readApiResponse } from '../src/services/apiResponse.ts';

const originalFetch = globalThis.fetch;
afterEach(() => { globalThis.fetch = originalFetch; });

for (const [name, body, status, expected] of [
  ['JSON credentials', '{"error":"Invalid email or password"}', 401, 'Invalid email or password'],
  ['legacy SQL plaintext', 'sql: no rows in result set', 401, 'Invalid email or password'],
  ['legacy SQL JSON', '{"error":"sql: no rows in result set"}', 401, 'Invalid email or password'],
  ['HTML error', '<html>private server exception</html>', 500, 'Server error. Please try again later.'],
  ['JSON internal error', '{"error":"private database detail"}', 500, 'Server error. Please try again later.'],
  ['empty error', '', 401, 'Invalid email or password'],
  ['malformed JSON', '{"error":', 401, 'Invalid email or password'],
  ['wrong error type', '{"error":{"detail":"private"}}', 401, 'Invalid email or password'],
  ['inactive account', '{"error":"Account is inactive. Contact your school administrator"}', 403, 'Account is inactive. Contact your school administrator'],
]) {
  test(`login handles ${name}`, async () => {
    globalThis.fetch = async () => new Response(body, { status });
    await assert.rejects(login({ email: 'user@example.com', password: 'example' }),
      (error) => error.message === expected && !(error instanceof SyntaxError));
  });
}

test('login preserves successful response', async () => {
  const response = { message: 'login successful', token: 'test-token',
    user: { id: 'test-id', email: 'user@example.com', role: 'ADMIN' } };
  globalThis.fetch = async () => new Response(JSON.stringify(response), { status: 200 });
  assert.deepEqual(await login({ email: 'user@example.com', password: 'example' }), response);
});

test('invalid successful JSON gives a safe message', async () => {
  await assert.rejects(readApiResponse(new Response('<html>unexpected</html>'), 'Request failed'),
    { message: 'Invalid response from server. Please try again.' });
});

test('non-JSON client errors use the endpoint fallback', async () => {
  await assert.rejects(readApiResponse(new Response('private SQL', { status: 400 }), 'Failed to create user'),
    { message: 'Failed to create user' });
});

test('JSON validation errors remain meaningful', async () => {
  await assert.rejects(readApiResponse(new Response('{"error":"email is required"}', { status: 400 }), 'Request failed'),
    { message: 'email is required' });
});
