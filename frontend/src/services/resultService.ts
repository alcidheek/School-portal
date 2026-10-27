import { authenticatedFetch } from './authSession.ts';
import { readApiResponse } from './apiResponse.ts';

export type Reference = { id: string; name: string };
export type Period = { class_id: string; academic_session_id: string; term_id: string; subject_id?: string };
export type Options = {
  sessions: Reference[];
  terms: (Reference & { academic_session_id: string })[];
  assignments: { class_id: string; subject_id: string; academic_session_id: string; term_id: string; class_name: string; subject_name: string }[];
};
export type Score = { id: string; student: Reference; subject: Reference; ca_score: number; exam_score: number; total_score: number; grade: string; remark: string };
export type Review = {
  batches: (Period & { subject_id: string; subject_name: string; status: string; rejection_reason: string | null; submitted_at: string | null; reviewed_at: string | null })[];
  results: Score[];
  students: Reference[];
  events: { subject_id: string; action: string; actor_id: string; reason: string | null; occurred_at: string }[];
  publication: { published_at: string; published_by: string } | null;
};
export async function resultRequest<T>(path: string, method = 'GET', data?: object): Promise<T> {
  const token = localStorage.getItem('token');
  if (!token) throw new Error('Please sign in again.');
  const response = await authenticatedFetch(`http://localhost:8080/api/v1/${path}`, {
    method, headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    ...(data ? { body: JSON.stringify(data) } : {}),
  });
  if (response.ok && response.status === 204) return undefined as T;
  return readApiResponse<T>(response, 'Unable to process results. Please try again.');
}
export function periodQuery(period: Period) {
  return new URLSearchParams(Object.entries(period).filter(([, value]) => Boolean(value)) as [string, string][]).toString();
}
export function editableBatch(review: Review | null) {
  return !!review && !review.publication && (review.batches.length === 0 || ['draft', 'rejected', 'missing'].includes(review.batches[0].status));
}
