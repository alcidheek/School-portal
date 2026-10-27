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
        for (const extension of ['.ts', '.tsx']) { try { return next(specifier + extension, context); } catch { /* Try next extension. */ } }
      }
      throw error;
    }
  },
  load(url, context, next) {
    if (url.endsWith('.css')) return { format: 'module', source: 'export default {};', shortCircuit: true };
    if (url.endsWith('.tsx')) return { format: 'module', shortCircuit: true, source: ts.transpileModule(readFileSync(new URL(url), 'utf8'), { compilerOptions: { jsx: ts.JsxEmit.ReactJSX, module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText };
    return next(url, context);
  },
});
const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost:5173' });
for (const key of ['window', 'document', 'localStorage', 'HTMLElement', 'Event']) globalThis[key] = dom.window[key];
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true });
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
const { act, createElement } = await import('react');
const { createRoot } = await import('react-dom/client');
const { MemoryRouter, Routes, Route } = await import('react-router-dom');
const { default: Workflow } = await import('../src/pages/results/ResultWorkflow.tsx');
const { default: Offerings } = await import('../src/pages/admin/ClassSubjects.tsx');
const { default: Published } = await import('../src/pages/results/PublishedResults.tsx');
const { default: Guard } = await import('../src/components/RoleGuard.tsx');
const api = await import('../src/services/resultService.ts');
const originalFetch = globalThis.fetch;
let root, requests, state;
const options = { sessions: [{ id: 'session', name: '2026/2027' }], terms: [{ id: 'term', name: 'FIRST_TERM', academic_session_id: 'session' }], assignments: [{ class_id: 'class', subject_id: 'math', academic_session_id: 'session', term_id: 'term', class_name: 'JSS 1', subject_name: 'Mathematics' }] };
const student = { id: 'student', name: 'Ada' };
function review(status = 'draft', withScore = false, published = false) {
  return { batches: [{ subject_id: 'math', subject_name: 'Mathematics', status, rejection_reason: status === 'rejected' ? 'Correct totals' : null, submitted_at: null, reviewed_at: null }], students: [student], results: withScore ? [{ id: 'score', student, subject: { id: 'math', name: 'Mathematics' }, ca_score: 30, exam_score: 50, total_score: 80, grade: 'A', remark: 'Excellent' }] : [], events: [], publication: published ? { published_at: '2026-10-10T12:00:00Z', published_by: 'admin' } : null };
}
function json(value, status = 200) { return new Response(JSON.stringify(value), { status }); }
async function mount(component, responder = () => json({}), initial = review(), path = '/') {
  localStorage.setItem('token', 'token'); state = initial; requests = [];
  globalThis.fetch = async (url, init) => {
    requests.push({ url, init });
    if (url.endsWith('result-options')) return json(options);
    if (url.endsWith('admin/classes')) return json([{ id: 'class', name: 'JSS 1' }]);
    if (url.endsWith('admin/subjects')) return json([{ id: 'math', name: 'Mathematics' }]);
    if (url.includes('result-batches?')) return json(state);
    return responder(url, init);
  };
  document.body.innerHTML = '<div id="root"></div>'; root = createRoot(document.getElementById('root'));
  await act(async () => root.render(createElement(MemoryRouter, { initialEntries: [path] }, component)));
}
async function select(index, value) { await act(async () => { const element = document.querySelectorAll('select')[index]; element.value = value; element.dispatchEvent(new Event('change', { bubbles: true })); }); }
async function input(element, value) { await act(async () => { const proto = element.tagName === 'TEXTAREA' ? dom.window.HTMLTextAreaElement.prototype : dom.window.HTMLInputElement.prototype; Object.getOwnPropertyDescriptor(proto, 'value').set.call(element, value); element.dispatchEvent(new Event('input', { bubbles: true })); }); }
function button(text) { return [...document.querySelectorAll('button')].find(b => b.textContent === text); }
async function click(text) { await act(async () => button(text).click()); }
async function loadWorkflow(teacher = true) { await select(0, 'session'); await select(2, 'term'); await select(1, teacher ? 'class/math' : 'class'); await click('Load results'); }
afterEach(async () => { if (root) await act(async () => root.unmount()); root = undefined; localStorage.clear(); globalThis.fetch = originalFetch; });

test('teacher saves a draft with scores and assigned batch identity', async () => {
  await mount(createElement(Workflow, { teacher: true }), (url, init) => { assert.ok(url.endsWith('teacher/results')); assert.deepEqual(JSON.parse(init.body), { class_id: 'class', academic_session_id: 'session', term_id: 'term', subject_id: 'math', student_id: 'student', ca_score: 30, exam_score: 50 }); state = review('draft', true); return json({}); });
  await loadWorkflow(); assert.equal(button('Submit full batch').disabled, true);
  await input(document.querySelector('[aria-label="CA for Ada"]'), '30'); await input(document.querySelector('[aria-label="Exam for Ada"]'), '50');
  await act(async () => document.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
  assert.match(document.body.textContent, /Draft saved/); assert.equal(button('Submit full batch').disabled, false);
});
test('teacher submits a complete batch and score controls become locked', async () => {
  await mount(createElement(Workflow, { teacher: true }), () => { state = review('submitted', true); return json({ status: 'submitted' }); }, review('draft', true));
  await loadWorkflow(); await click('Submit full batch'); assert.equal(document.querySelector('form fieldset').disabled, true);
  assert.equal(requests.filter(r => r.init.method === 'POST').length, 1); assert.match(document.body.textContent, /Full batch submitted/);
});
test('rejected teacher batch displays reason, permits corrections and resubmission', async () => {
  await mount(createElement(Workflow, { teacher: true }), (url, init) => { assert.equal(init.method, 'PATCH'); state = review('rejected', true); return new Response(null, { status: 204 }); }, review('rejected', true));
  await loadWorkflow(); assert.match(document.body.textContent, /Correct totals/); assert.equal(document.querySelector('form fieldset').disabled, false); assert.equal(button('Resubmit full batch').disabled, false);
  await input(document.querySelector('[aria-label="CA for Ada"]'), '35'); await act(async () => document.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
  assert.ok(requests.some(r => r.url.endsWith('teacher/results/score')));
});
test('teacher server errors are shown and selecting another period clears scores', async () => {
  await mount(createElement(Workflow, { teacher: true }), () => json({ error: 'missing results for Ada' }, 409), review('draft', true));
  await loadWorkflow(); await click('Submit full batch'); assert.match(document.querySelector('[role="alert"]').textContent, /missing results/);
  await select(0, ''); assert.equal(document.querySelector('form'), null);
});
test('admin reviews full student batch, rejects with reason and sees updated status', async () => {
  await mount(createElement(Workflow), (url, init) => { assert.ok(url.endsWith('/reject')); assert.equal(JSON.parse(init.body).reason, 'Correct totals'); state = review('rejected', true); return json({ status: 'rejected' }); }, review('submitted', true));
  await loadWorkflow(false); assert.match(document.querySelector('table').textContent, /Ada305080AExcellent/);
  await input(document.querySelector('textarea'), 'Correct totals'); await act(async () => document.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })));
  assert.match(document.body.textContent, /Batch rejected/); assert.equal(button('Publish complete class-term').disabled, true);
});
test('admin cannot publish incomplete subjects and uses whole class-term payload', async () => {
  await mount(createElement(Workflow), (url, init) => { assert.ok(url.endsWith('result-publications')); const body = JSON.parse(init.body); assert.deepEqual(body, { class_id: 'class', academic_session_id: 'session', term_id: 'term' }); state = review('approved', true, true); return json({}); }, review('submitted', true));
  await loadWorkflow(false); assert.equal(button('Publish complete class-term').disabled, true);
  state = review('approved', true); await click('Load results'); await click('Publish complete class-term'); assert.equal(button('Publish complete class-term').disabled, true); assert.match(document.body.textContent, /Entire class-term published/);
});
test('admin publication failure retains unpublished state and explains missing subject', async () => {
  await mount(createElement(Workflow), () => json({ error: 'Science: missing results for Ada' }, 409), review('approved', true));
  await loadWorkflow(false); await click('Publish complete class-term'); assert.match(document.querySelector('[role="alert"]').textContent, /Science: missing/); assert.equal(button('Publish complete class-term').disabled, false);
});
function configuration(subjects = [], locked = false) { return { locked, subjects, students: [], class_teacher: null, subject_teachers: [], candidates: [{ id: 'student', name: 'Ada', role: 'STUDENT' }, { id: 'teacher', name: 'Mr Obi', role: 'TEACHER' }] }; }
async function loadConfiguration() { await select(0, 'session'); await select(1, 'term'); await select(2, 'class'); await click('Load configuration'); }
test('term configuration assigns and removes subjects with explicit term identity', async () => {
  let subjects = []; await mount(createElement(Offerings), (url, init) => { if (init.method === 'PATCH') { const body = JSON.parse(init.body); assert.equal(body.term_id, 'term'); assert.equal(body.academic_session_id, 'session'); assert.equal(body.category, 'offerings'); subjects = body.operation === 'add' ? [{ id: 'math', name: 'Mathematics' }] : []; } return json(configuration(subjects)); });
  await loadConfiguration(); await select(5, 'math'); await click('Assign subject'); assert.ok(button('Remove Mathematics')); await click('Remove Mathematics'); assert.equal(button('Remove Mathematics'), undefined);
});
test('frozen offering changes show API conflict without dropping the existing offering', async () => {
  await mount(createElement(Offerings), (url, init) => init.method === 'GET' ? json(configuration([{ id: 'math', name: 'Mathematics' }])) : json({ error: 'Configuration is frozen after first submission' }, 409));
  await loadConfiguration(); await click('Remove Mathematics'); assert.match(document.querySelector('[role="alert"]').textContent, /frozen/); assert.ok(button('Remove Mathematics'));
});
test('term roster enrollment and teacher replacements carry the selected term', async () => {
  await mount(createElement(Offerings), (url, init) => { if (init.method === 'PATCH') { const body = JSON.parse(init.body); assert.equal(body.term_id, 'term'); assert.equal(body.class_id, 'class'); } return json(configuration([{ id: 'math', name: 'Mathematics' }])); });
  await loadConfiguration(); await select(4, 'student'); await click('Enroll student');
  await select(6, 'teacher'); await click('Set class teacher'); await select(7, 'math'); await click('Set subject teacher');
  assert.deepEqual(requests.filter(r => r.init.method === 'PATCH').map(r => JSON.parse(r.init.body).category), ['enrollment', 'class_teachers', 'subject_teachers']);
});
test('locked configuration disables roster subject teacher and initialization mutations', async () => {
  await mount(createElement(Offerings), () => json(configuration([{ id: 'math', name: 'Mathematics' }], true)));
  await loadConfiguration(); assert.match(document.body.textContent, /locked after submission/);
  for (const text of ['Enroll student', 'Assign subject', 'Set class teacher', 'Set subject teacher', 'Copy selected configuration']) assert.equal(button(text).closest('fieldset').disabled, true);
});
test('changing term clears the loaded configuration and student choices', async () => {
  await mount(createElement(Offerings), () => json(configuration([{ id: 'math', name: 'Mathematics' }])));
  await loadConfiguration(); assert.ok(button('Remove Mathematics')); await select(1, ''); assert.equal(button('Remove Mathematics'), undefined); assert.equal(button('Load configuration').disabled, true);
});
test('teacher assignment choices require the selected authorized term', async () => {
  await mount(createElement(Workflow, { teacher: true })); await select(0, 'session');
  assert.equal(document.querySelectorAll('select')[1].options.length, 1);
  await select(2, 'term'); assert.equal(document.querySelectorAll('select')[1].options.length, 2);
  await select(1, 'class/math'); await select(2, ''); assert.equal(document.querySelectorAll('select')[1].value, ''); assert.equal(button('Load results').disabled, true);
});
test('administrator copies selected categories and sees returned counts without losing the loaded term', async () => {
  options.terms.push({ id: 'source', name: 'THIRD_TERM', academic_session_id: 'session' });
  try {
    await mount(createElement(Offerings), (url, init) => {
      if (url.endsWith('term-initializations')) {
        assert.deepEqual(JSON.parse(init.body), { academic_session_id: 'session', term_id: 'term', source_term_id: 'source', categories: ['enrollment', 'offerings', 'class_teachers'] });
        return json({ copied: { enrollment: 12, offerings: 5, class_teachers: 2 } });
      }
      return json(configuration());
    });
    await loadConfiguration(); await select(3, 'source');
    await act(async () => document.querySelectorAll('input[type="checkbox"]')[3].click());
    await click('Copy selected configuration'); assert.match(document.body.textContent, /Student roster 12, Subject offerings 5, Class teachers 2/);
    assert.equal(requests.filter(r => r.url.endsWith('term-initializations')).length, 1);
  } finally { options.terms.pop(); }
});
test('student page renders only returned published scores and handles unpublished empty list', async () => {
  let results = []; await mount(createElement(Published), () => json({ results }));
  await select(0, 'session'); await select(1, 'term'); await click('View published results'); assert.match(document.body.textContent, /No published results/);
  results = review('approved', true).results; await click('View published results'); assert.match(document.querySelector('table').textContent, /Mathematics305080AExcellent/);
  assert.equal(requests.find(r => r.url.includes('student/results?')).url.includes('student_id'), false);
});
test('role guard uses live account role and rejects a mismatched account', async () => {
  await mount(createElement(Routes, null, createElement(Route, { element: createElement(Guard, { role: 'ADMIN' }) }, createElement(Route, { path: '/', element: createElement('p', null, 'Protected') })), createElement(Route, { path: '*', element: createElement('p', null, 'Login') })), () => json({ role: 'ADMIN' }));
  assert.match(document.body.textContent, /Protected/); await act(async () => root.unmount()); root = undefined;
  await mount(createElement(Routes, null, createElement(Route, { path: '/protected', element: createElement(Guard, { role: 'ADMIN' }) }, createElement(Route, { index: true, element: createElement('p', null, 'Protected') })), createElement(Route, { path: '/', element: createElement('p', null, 'Login') })), () => json({ role: 'TEACHER' }), review(), '/protected');
  assert.match(document.body.textContent, /Login/);
});
test('rapid teacher submissions issue a single mutation and disable controls while pending', async () => {
  let finish; await mount(createElement(Workflow, { teacher: true }), () => new Promise(resolve => { finish = resolve; }), review('draft', true));
  await loadWorkflow(); await act(async () => { button('Submit full batch').click(); button('Submit full batch').click(); });
  assert.equal(requests.filter(r => r.init.method === 'POST').length, 1); assert.equal(document.querySelector('fieldset').disabled, true);
  await act(async () => { state = review('submitted', true); finish(json({})); });
});
test('API client handles 204 and preserves safe errors; query omits absent subject', async () => {
  localStorage.setItem('token', 'token'); globalThis.fetch = async () => new Response(null, { status: 204 }); assert.equal(await api.resultRequest('teacher/results/score', 'PATCH', {}), undefined);
  globalThis.fetch = async () => json({ error: 'private SQL details' }, 500); await assert.rejects(api.resultRequest('admin/result-publications', 'POST', {}), /Server error/);
  assert.equal(api.periodQuery({ class_id: 'class', academic_session_id: 'session', term_id: 'term' }), 'class_id=class&academic_session_id=session&term_id=term');
  assert.equal(api.editableBatch(review('approved', true)), false); assert.equal(api.editableBatch(review('rejected', true)), true); assert.equal(api.editableBatch(review('rejected', true, true)), false);
});
