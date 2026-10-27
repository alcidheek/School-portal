import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { registerHooks } from 'node:module';
import { afterEach, test } from 'node:test';
import ts from 'typescript';
import { JSDOM } from 'jsdom';

registerHooks({
  resolve(specifier, context, next) {
    try { return next(specifier, context); }
    catch (error) {
      if (specifier.startsWith('.') && !/\.[a-z]+$/i.test(specifier)) {
        for (const extension of ['.ts', '.tsx']) {
          try { return next(specifier + extension, context); } catch { /* Try TSX. */ }
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
const { default: Forgot } = await import('../src/pages/ForgotPasswordPage.tsx');
const { default: Reset } = await import('../src/pages/ResetPasswordPage.tsx');
const { default: Login } = await import('../src/pages/LoginPage.tsx');
const { default: LogoutButton } = await import('../src/components/LogoutButton.tsx');
const { default: Monitor } = await import('../src/components/SessionMonitor.tsx');
const { default: Toast } = await import('../src/components/Toast.tsx');
const auth = await import('../src/services/authService.ts');
const { authenticatedFetch } = await import('../src/services/authSession.ts');
const originalFetch = globalThis.fetch;
let root;
let requests;
const json = (value, status = 200) => new Response(JSON.stringify(value), { status });
async function mount(path, respond) {
  requests = [];
  globalThis.fetch = async (url, init) => { requests.push({ url, init }); return respond(url, init); };
  document.body.innerHTML = '<div id="root"></div>';
  root = createRoot(document.getElementById('root'));
  await act(async () => root.render(createElement(MemoryRouter, { initialEntries: [path] },
    createElement(Monitor), createElement(Routes, null,
      createElement(Route, { path: '/forgot-password', element: createElement(Forgot) }),
      createElement(Route, { path: '/reset-password', element: createElement(Reset) }),
      createElement(Route, { path: '/', element: createElement(Login) }),
      createElement(Route, { path: '/dashboard', element: createElement(LogoutButton) }),
    ))));
}
async function input(id, value) {
  await act(async () => {
    const element = document.getElementById(id);
    Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, 'value').set.call(element, value);
    element.dispatchEvent(new dom.window.Event('input', { bubbles: true }));
  });
}
async function submit() {
  await act(async () => document.querySelector('form').dispatchEvent(new dom.window.Event('submit', { bubbles: true, cancelable: true })));
}
async function fillReset(code = '012345', password = 'ResetStrongPassword2!', confirmation = password) {
  await input('reset-email', ' USER@Example.com '); await input('reset-code', code);
  await input('reset-password', password); await input('reset-confirmation', confirmation);
}
function seedSession() { localStorage.setItem('token', 'test-token'); localStorage.setItem('user', '{"role":"ADMIN"}'); }
afterEach(async () => {
  if (root) await act(async () => root.unmount());
  root = undefined; localStorage.clear(); globalThis.fetch = originalFetch;
});

test('forgot form validates email and normalizes the request without enumerating accounts', async () => {
  await mount('/forgot-password', () => json({ message: 'If the account exists, a code has been emailed.' }));
  await input('reset-email', 'invalid'); await submit(); assert.equal(requests.length, 0);
  assert.match(document.querySelector('[role="alert"]').textContent, /valid email/);
  await input('reset-email', ' USER@Example.com '); await submit();
  assert.deepEqual(JSON.parse(requests[0].init.body), { email: 'user@example.com' });
  assert.match(document.querySelector('[role="status"]').textContent, /If the account exists/);
  assert.equal(document.querySelector('a[href="/"]').textContent, 'Back to login');
  assert.equal(localStorage.length, 0);
});

test('forgot form handles network and non-JSON failures safely', async () => {
  await mount('/forgot-password', () => { throw new Error('private network details'); });
  await input('reset-email', 'user@example.com'); await submit();
  assert.match(document.querySelector('[role="alert"]').textContent, /Unable to connect/);
  globalThis.fetch = async () => new Response('sql: private details', { status: 500 }); await submit();
  assert.match(document.querySelector('[role="alert"]').textContent, /Server error/);
  assert.doesNotMatch(document.body.textContent, /private|sql:/);
});

for (const path of ['/forgot-password', '/reset-password']) {
  test(`${path} prevents rapid duplicate requests and disables inputs while pending`, async () => {
    let resolve;
    await mount(path, () => new Promise(done => { resolve = done; }));
    if (path === '/reset-password') await fillReset(); else await input('reset-email', 'user@example.com');
    await submit(); await submit(); assert.equal(requests.length, 1);
    for (const control of document.querySelectorAll('input,button')) assert.equal(control.disabled, true);
    await act(async () => resolve(json({ message: 'Success' })));
  });
}

test('reset form validates email, code, password policy and matching passwords locally', async () => {
  await mount('/reset-password', () => { throw new Error('Must not call API'); });
  await submit(); assert.match(document.querySelector('[role="alert"]').textContent, /valid email/);
  await fillReset('12345'); await submit(); assert.match(document.querySelector('[role="alert"]').textContent, /six-digit/);
  await fillReset('012345', 'weak'); await submit(); assert.match(document.querySelector('[role="alert"]').textContent, /12 characters/);
  await fillReset('012345', 'ResetStrongPassword2!', 'OtherStrongPassword3!'); await submit();
  assert.match(document.querySelector('[role="alert"]').textContent, /do not match/); assert.equal(requests.length, 0);
  assert.equal(localStorage.length, 0);
});

test('reset success clears cached authentication and sensitive form and returns to login', async () => {
  seedSession(); await mount('/reset-password', () => json({ message: 'Password reset successfully.' }));
  await fillReset(); await submit();
  assert.deepEqual(JSON.parse(requests[0].init.body), { email: 'user@example.com', code: '012345', new_password: 'ResetStrongPassword2!' });
  assert.match(document.body.textContent, /Please sign in with your new password/);
  assert.equal(document.getElementById('reset-code'), null); assert.equal(localStorage.length, 0);
  assert.equal(document.getElementById('password').value, '');
});

for (const message of ['Invalid or expired reset code', 'Reset code already used', 'Reset verification is temporarily unavailable. Try again later.']) {
  test(`reset form shows safe rejection: ${message}`, async () => {
    await mount('/reset-password', () => json({ error: message }, 400)); await fillReset(); await submit();
    assert.equal(document.querySelector('[role="alert"]').textContent, message);
    assert.equal(document.querySelector('button').disabled, false); assert.equal(localStorage.length, 0);
  });
}

test('login forgot-password link navigates to the real reset request page', async () => {
  await mount('/', () => { throw new Error('Must not call API'); });
  await act(async () => document.querySelector('.forgot-password a').dispatchEvent(new dom.window.MouseEvent('click', { bubbles: true, cancelable: true, button: 0 })));
  assert.match(document.body.textContent, /Enter your registered email/);
});

for (const outcome of ['success', 'expired', 'network failure', 'server failure']) {
  test(`logout ${outcome} clears authentication and navigates to login`, async () => {
    seedSession(); await mount('/dashboard', () => {
      if (outcome === 'network failure') throw new Error('Network unavailable');
      return json(outcome === 'success' ? { message: 'Signed out' } : { error: 'safe failure' }, outcome === 'expired' ? 401 : outcome === 'server failure' ? 500 : 200);
    });
    await act(async () => document.querySelector('button').click());
    assert.equal(requests[0].url, 'http://localhost:8080/auth/logout');
    assert.equal(requests[0].init.headers.Authorization, 'Bearer test-token');
    assert.equal(requests[0].init.method, 'POST'); assert.equal(localStorage.length, 0);
    assert.match(document.body.textContent, /Welcome Back/);
    const toast = document.querySelector('.notification-toast');
    const warning = outcome === 'network failure' || outcome === 'server failure';
    assert.ok(toast.classList.contains(warning ? 'notification-toast--warning' : 'notification-toast--success'));
    assert.match(toast.textContent, warning ? /Server logout could not be confirmed/ : /Signed out successfully/);
    assert.equal(document.querySelector('.login-card .notification-toast'), null);
    assert.doesNotMatch(document.querySelector('.login-card').textContent, /Signed out/);
  });
}

for (const [status, message, invalid] of [[401, 'Authentication is required', true], [403, 'Account is inactive. Contact your school administrator', true], [403, 'Access to this resource is forbidden', false], [401, 'Current password is incorrect', false]]) {
  test(`authenticated client ${status} ${message} handles session state correctly`, async () => {
    seedSession(); await mount('/dashboard', () => json({ error: message }, status));
    await act(async () => authenticatedFetch('/api/v1/me', { headers: { Authorization: 'Bearer test-token' } }));
    assert.equal(localStorage.getItem('token') === null, invalid);
    assert.equal(document.body.textContent.includes('Welcome Back'), invalid);
  });
}

test('reset API rejects malformed success without clearing an existing session', async () => {
  seedSession(); globalThis.fetch = async () => json({});
  await assert.rejects(auth.resetPassword('user@example.com', '012345', 'ResetStrongPassword2!'), /Invalid response/);
  assert.equal(localStorage.getItem('token'), 'test-token');
});

test('logout toast stays outside the login card and automatically dismisses at four seconds', async (context) => {
  context.mock.timers.enable({ apis: ['setTimeout'] });
  await mount({ pathname: '/', state: { toast: { message: 'Signed out successfully.', kind: 'success' } } }, () => { throw new Error('Must not call API'); });
  const card = document.querySelector('.login-card');
  const originalCard = card.innerHTML;
  assert.equal(document.querySelector('.notification-toast').parentElement, document.body);
  assert.equal(card.firstElementChild.className, 'school-header');
  await act(async () => context.mock.timers.tick(3999));
  assert.ok(document.querySelector('.notification-toast'));
  await act(async () => context.mock.timers.tick(1));
  assert.equal(document.querySelector('.notification-toast'), null);
  assert.equal(card.innerHTML, originalCard);
});

test('warning toast is accessible and can be dismissed manually', async () => {
  await mount({ pathname: '/', state: { toast: { message: 'Server logout could not be confirmed.', kind: 'warning' } } }, () => { throw new Error('Must not call API'); });
  assert.equal(document.querySelector('.notification-toast').getAttribute('role'), 'alert');
  await act(async () => document.querySelector('[aria-label="Dismiss notification"]').click());
  assert.equal(document.querySelector('.notification-toast'), null);
  assert.ok(document.querySelector('.login-card'));
});

test('reusable toast supports error styling and cleans up its timer on unmount', async (context) => {
  const clear = context.mock.method(window, 'clearTimeout');
  document.body.innerHTML = '<div id="root"></div>';
  root = createRoot(document.getElementById('root'));
  await act(async () => root.render(createElement(Toast, { message: 'Unable to complete the request.', kind: 'error' })));
  const toast = document.querySelector('.notification-toast--error');
  assert.equal(toast.getAttribute('role'), 'alert');
  await act(async () => root.unmount()); root = undefined;
  assert.equal(document.querySelector('.notification-toast'), null);
  assert.equal(clear.mock.callCount(), 1);
});
