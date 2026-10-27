import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { resultRequest, type Options, type Score } from '../../services/resultService.ts';
import './Results.css';

export default function PublishedResults() {
  const [options, setOptions] = useState<Options>({ sessions: [], terms: [], assignments: [] });
  const [session, setSession] = useState('');
  const [term, setTerm] = useState('');
  const [results, setResults] = useState<Score[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => {
    let active = true;
    resultRequest<Options>('student/result-options').then(o => { if (active) setOptions(o); }).catch((e: unknown) => { if (active) setError(e instanceof Error ? e.message : 'Unable to load periods.'); });
    return () => { active = false; };
  }, []);
  async function load() {
    setBusy(true); setError(''); setResults(null);
    try { const response = await resultRequest<{ results: Score[] }>(`student/results?academic_session_id=${session}&term_id=${term}`); setResults(response.results); }
    catch (e) { setError(e instanceof Error ? e.message : 'Unable to load results.'); }
    finally { setBusy(false); }
  }
  return <main className="result-workspace"><h1>My published results</h1><Link to="/student/dashboard">Student dashboard</Link>
    {error && <p role="alert">{error}</p>}
    <fieldset disabled={busy} className="result-card result-filters"><label>Academic session<select value={session} onChange={e => { setSession(e.target.value); setTerm(''); setResults(null); }}><option value="">Select session</option>{options.sessions.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></label>
      <label>Term<select value={term} onChange={e => { setTerm(e.target.value); setResults(null); }}><option value="">Select term</option>{options.terms.filter(t => t.academic_session_id === session).map(t => <option key={t.id} value={t.id}>{t.name.replaceAll('_', ' ')}</option>)}</select></label><button disabled={!session || !term} onClick={() => void load()}>View published results</button></fieldset>
    {results && (results.length === 0 ? <p role="status">No published results are available for this term.</p> : <div className="result-card result-table-scroll"><table><thead><tr><th>Subject</th><th>CA</th><th>Exam</th><th>Total</th><th>Grade</th><th>Remark</th></tr></thead><tbody>{results.map(r => <tr key={r.id}><td>{r.subject.name}</td><td>{r.ca_score}</td><td>{r.exam_score}</td><td>{r.total_score}</td><td>{r.grade}</td><td>{r.remark}</td></tr>)}</tbody></table></div>)}
  </main>;
}
