import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { registerHooks } from 'node:module';
import { afterEach, test } from 'node:test';
import ts from 'typescript';
import { JSDOM } from 'jsdom';

// Transpile application TSX in memory and ignore style imports. These tests
// exercise the real React forms and router, using a mocked HTTP boundary.
registerHooks({
  resolve(specifier, context, next) {
    try { return next(specifier, context); }
    catch (error) {
      if (specifier.startsWith('.') && !/\.[a-z]+$/i.test(specifier)) {
        for (const extension of ['.ts', '.tsx']) {
          try { return next(specifier + extension, context); } catch { /* Try next supported extension. */ }
        }
      }
      throw error;
    }
  },
  load(url, context, next) {
    if (url.endsWith('.css')) return { format: 'module', source: 'export default {};', shortCircuit: true };
    if (url.endsWith('.tsx')) return {
      format: 'module', shortCircuit: true,
      source: ts.transpileModule(readFileSync(new URL(url), 'utf8'), {
        compilerOptions: { jsx: ts.JsxEmit.ReactJSX, module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
      }).outputText,
    };
    return next(url, context);
  },
});
const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost/' });
globalThis.window = dom.window;
globalThis.document = dom.window.document;
globalThis.localStorage = dom.window.localStorage;
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
const { createElement, act } = await import('react');
const { createRoot } = await import('react-dom/client');
const { MemoryRouter, Routes, Route } = await import('react-router-dom');
const { default: AccountSecurity } = await import('../src/pages/admin/AccountSecurity.tsx');
const { default: LoginPage } = await import('../src/pages/LoginPage.tsx');
const { default: AddUserPage } = await import('../src/pages/admin/AddUserPage.tsx');
const api = await import('../src/services/accountSecurityService.ts');
const originalFetch = globalThis.fetch;
let root;
let requests;
const admin = { id: 'admin-id', email: 'old@example.com', role: 'ADMIN' };
function json(value, status = 200) { return new Response(JSON.stringify(value), { status }); }
async function mount(response, options = {}) {
  const cachedUser = options.cachedUser === undefined ? admin : options.cachedUser;
  if (cachedUser) { localStorage.setItem('token', 'old-token'); localStorage.setItem('user', JSON.stringify(cachedUser)); }
  requests = [];
  globalThis.fetch = async (url, init) => {
    requests.push({ url, init });
    if (init.method === 'GET') return options.accountResponse ?? json({ user_id: admin.id, email: admin.email, role: 'ADMIN' });
    return response(url, init);
  };
  document.body.innerHTML = '<div id="root"></div>';
  root = createRoot(document.getElementById('root'));
  await act(async () => root.render(createElement(MemoryRouter, { initialEntries: [options.addUser ? '/admin/users/new?role=TEACHER' : '/admin/settings/account-security'] },
    createElement(Routes, null,
      createElement(Route, { path: '/admin/settings/account-security', element: createElement(AccountSecurity) }),
      createElement(Route, { path: '/', element: createElement(LoginPage) }),
      createElement(Route, { path: '/admin/users/new', element: createElement(AddUserPage) }),
      createElement(Route, { path: '/admin/users', element: createElement('p', null, 'Users list') }),
    ))));
}
async function input(id, value) {
  await act(async () => {
    const element = document.getElementById(id);
    Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, 'value').set.call(element, value);
    element.dispatchEvent(new dom.window.Event('input', { bubbles: true }));
  });
}
async function submit(index = 0) {
  await act(async () => document.querySelectorAll('form')[index].dispatchEvent(new dom.window.Event('submit', { bubbles: true, cancelable: true })));
}
function assertNoStoredPasswords() {
  for (let i = 0; i < localStorage.length; i++) {
    assert.doesNotMatch(localStorage.getItem(localStorage.key(i)), /OriginalStrong|NewStrong|current_password|new_password|confirm_password/);
  }
}
afterEach(async () => {
  if (root) await act(async () => root.unmount());
  root = undefined; localStorage.clear(); globalThis.fetch = originalFetch;
});

test('email success updates displayed email, token and cache and clears sensitive forms', async () => {
  await mount(() => json({ message: 'Email updated successfully', token: 'new-token', user: { ...admin, email: 'new@example.com' } }));
  await input('new-email', ' NEW@Example.com '); await input('email-password', 'OriginalStrong1!'); await submit();
  assert.equal(document.querySelector('[data-testid="current-email"]').textContent, 'new@example.com');
  assert.match(document.body.textContent, /Email updated successfully/);
  assert.equal(document.getElementById('email-password').value, '');
  assert.equal(document.getElementById('new-email').value, '');
  assert.equal(localStorage.getItem('token'), 'new-token');
  assert.equal(JSON.parse(localStorage.getItem('user')).email, 'new@example.com');
  assert.deepEqual(JSON.parse(requests[1].init.body), { new_email: 'new@example.com', current_password: 'OriginalStrong1!' });
  assert.equal(requests[1].init.headers.Authorization, 'Bearer old-token');
  assertNoStoredPasswords();
});
test('email errors show safe API messages and clear passwords without replacing authentication', async () => {
  await mount(() => json({ error: 'Email already belongs to another account' }, 409));
  await input('new-email', 'taken@example.com'); await input('email-password', 'OriginalStrong1!'); await submit();
  assert.match(document.querySelector('[role="alert"]').textContent, /already belongs/);
  assert.equal(document.getElementById('email-password').value, '');
  assert.equal(localStorage.getItem('token'), 'old-token'); assertNoStoredPasswords();
});
test('password success clears authentication and shows sign-in-again message on login', async () => {
  await mount(() => json({ message: 'Password updated successfully. Please sign in again.' }));
  await input('current-password', 'OriginalStrong1!'); await input('new-password', 'NewStrongPassword2!'); await input('confirm-password', 'NewStrongPassword2!'); await submit(1);
  assert.equal(localStorage.getItem('token'), null); assert.equal(localStorage.getItem('user'), null);
  assert.match(document.body.textContent, /Password updated successfully. Please sign in again/);
  assert.match(document.body.textContent, /Welcome Back/);
  assert.equal(document.getElementById('password').value, ''); assertNoStoredPasswords();
});
test('weak and mismatched passwords are rejected locally and clear all sensitive inputs', async () => {
  await mount(() => { throw new Error('Should not call backend'); });
  await input('current-password', 'OriginalStrong1!'); await input('new-password', 'weak'); await input('confirm-password', 'weak'); await submit(1);
  assert.match(document.querySelector('[role="alert"]').textContent, /12 characters/);
  for (const id of ['current-password', 'new-password', 'confirm-password']) assert.equal(document.getElementById(id).value, '');
  await input('current-password', 'OriginalStrong1!'); await input('new-password', 'NewStrongPassword2!'); await input('confirm-password', 'different'); await submit(1);
  assert.match(document.querySelector('[role="alert"]').textContent, /do not match/);
  assert.equal(requests.length, 1); assertNoStoredPasswords();
});
test('both forms disable while pending and rapid submissions issue only one PATCH', async () => {
  let resolve;
  await mount(() => new Promise((done) => { resolve = done; }));
  await input('new-email', 'new@example.com'); await input('email-password', 'OriginalStrong1!');
  await act(async () => {
    const form = document.querySelector('form');
    form.dispatchEvent(new dom.window.Event('submit', { bubbles: true, cancelable: true }));
    form.dispatchEvent(new dom.window.Event('submit', { bubbles: true, cancelable: true }));
  });
  assert.equal(requests.filter((r) => r.init.method === 'PATCH').length, 1);
  for (const fieldset of document.querySelectorAll('fieldset')) assert.equal(fieldset.disabled, true);
  await act(async () => resolve(json({ error: 'Current password is incorrect' }, 401)));
  assert.equal(document.getElementById('email-password').value, '');
  for (const fieldset of document.querySelectorAll('fieldset')) assert.equal(fieldset.disabled, false);
});
test('non-JSON and internal errors are safely parsed by the credential API client', async () => {
  localStorage.setItem('token', 'token');
  globalThis.fetch = async () => new Response('sql: private failure', { status: 500 });
  await assert.rejects(api.changeEmail('new@example.com', 'OriginalStrong1!'), { message: 'Server error. Please try again later.' });
  globalThis.fetch = async () => new Response('<html>private</html>', { status: 400 });
  await assert.rejects(api.changePassword('OriginalStrong1!', 'NewStrongPassword2!', 'NewStrongPassword2!'), /Unable to update your account/);
  assertNoStoredPasswords();
});
test('cached non-admin and malformed authentication are rejected', () => {
  localStorage.setItem('token', 'token'); localStorage.setItem('user', JSON.stringify({ ...admin, role: 'TEACHER' }));
  assert.equal(api.storedAdmin(), null);
  localStorage.setItem('user', '{'); assert.equal(api.storedAdmin(), null);
  localStorage.removeItem('token'); assert.equal(api.storedAdmin(), null);
});
test('password byte limit matches bcrypt and strong policy accepts valid values', () => {
  assert.equal(api.strongPasswordError('NewStrongPassword2!'), '');
  assert.notEqual(api.strongPasswordError('Ab1!' + 'é'.repeat(35)), '');
});

test('unauthenticated and non-admin users are redirected away from the security page', async () => {
  await mount(() => { throw new Error('Must not submit'); }, { cachedUser: null });
  assert.match(document.body.textContent, /Welcome Back/); assert.equal(requests.length, 0);
  await act(async () => root.unmount()); root = undefined;
  await mount(() => { throw new Error('Must not submit'); }, { cachedUser: { ...admin, role: 'TEACHER' } });
  assert.match(document.body.textContent, /Welcome Back/); assert.equal(requests.length, 0);
});

test('live account rejection disables both credential forms and shows a safe error', async () => {
  await mount(() => { throw new Error('Must not submit'); }, { accountResponse: json({ error: 'Account is inactive. Contact your school administrator' }, 403) });
  assert.match(document.querySelector('[role="alert"]').textContent, /Account is inactive/);
  for (const fieldset of document.querySelectorAll('fieldset')) assert.equal(fieldset.disabled, true);
  await submit(); assert.equal(requests.length, 1); assertNoStoredPasswords();
});

test('password rejection clears fields and keeps the current session', async () => {
  await mount(() => json({ error: 'Current password is incorrect' }, 401));
  await input('current-password', 'OriginalStrong1!'); await input('new-password', 'NewStrongPassword2!'); await input('confirm-password', 'NewStrongPassword2!'); await submit(1);
  assert.match(document.querySelector('[role="alert"]').textContent, /Current password is incorrect/);
  for (const id of ['current-password', 'new-password', 'confirm-password']) assert.equal(document.getElementById(id).value, '');
  assert.equal(localStorage.getItem('token'), 'old-token'); assertNoStoredPasswords();
});

test('malformed success responses do not corrupt stored authentication', async () => {
  localStorage.setItem('token', 'old-token'); localStorage.setItem('user', JSON.stringify(admin));
  globalThis.fetch = async () => json(null);
  await assert.rejects(api.changeEmail('new@example.com', 'OriginalStrong1!'), /Invalid account response/);
  await assert.rejects(api.changePassword('OriginalStrong1!', 'NewStrongPassword2!', 'NewStrongPassword2!'), /Invalid account response/);
  assert.equal(localStorage.getItem('token'), 'old-token'); assertNoStoredPasswords();
});

test('registration normalizes email and name, omits teacher guardians and clears submitted passwords', async () => {
  await mount(() => json({ message: 'user successfully created' }, 201), { addUser: true });
  await input('fullName', ' Teacher '); await input('email', ' TEACHER@Example.com ');
  await input('phone', '1234567'); await input('password', 'OriginalStrong1!'); await submit();
  assert.match(document.body.textContent, /Users list/);
  assert.deepEqual(JSON.parse(requests[0].init.body), { full_name: 'Teacher', email: 'teacher@example.com', phone: '1234567', password: 'OriginalStrong1!', role: 'TEACHER' });
  assertNoStoredPasswords();
});

test('registration rejects blank names and bad phones locally and clears passwords', async () => {
  await mount(() => { throw new Error('Must not submit'); }, { addUser: true });
  await input('fullName', ' '); await input('email', 'teacher@example.com');
  await input('phone', 'bad'); await input('password', 'OriginalStrong1!'); await submit();
  assert.match(document.body.textContent, /Enter a full name/);
  assert.equal(document.getElementById('password').value, ''); assert.equal(requests.length, 0);
  await input('fullName', 'Teacher'); await input('password', 'OriginalStrong1!'); await submit();
  assert.match(document.body.textContent, /phone number/);
  assert.equal(document.getElementById('password').value, ''); assert.equal(requests.length, 0);
});
