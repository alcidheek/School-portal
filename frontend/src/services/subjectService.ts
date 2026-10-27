
import {
  getSubjects,
  type Subject,
} from "./adminService";
import { resultRequest } from "./resultService";

export { getSubjects };
export type { Subject };

export type SubjectRequest = {
  name: string;
};

export type ClassSubjectOfferingResponse = {
  subjects: Pick<Subject, "id" | "name">[];
};

export async function createSubject(
  data: SubjectRequest
): Promise<Subject> {
  return resultRequest<Subject>(
    "admin/subjects",
    "POST",
    data
  );
}

export async function updateSubject(
  subjectID: string,
  data: SubjectRequest
): Promise<Subject> {
  return resultRequest<Subject>(
    `admin/subjects/${subjectID}`,
    "PATCH",
    data
  );
}

export async function deleteSubject(
  subjectID: string
): Promise<void> {
  await resultRequest<void>(
    `admin/subjects/${subjectID}`,
    "DELETE"
  );
}

export async function getClassSubjectOfferings(
  classID: string,
  academicSessionID: string,
  termID: string
): Promise<ClassSubjectOfferingResponse> {
  const query = new URLSearchParams({
    academic_session_id: academicSessionID,
    term_id: termID,
  });

  return resultRequest<ClassSubjectOfferingResponse>(
    `admin/classes/${classID}/subjects?${query.toString()}`
  );
}

export async function addClassSubjectOffering(
  classID: string,
  subjectID: string,
  academicSessionID: string,
  termID: string
): Promise<ClassSubjectOfferingResponse> {
  return resultRequest<ClassSubjectOfferingResponse>(
    `admin/classes/${classID}/subjects`,
    "POST",
    {
      subject_id: subjectID,
      academic_session_id: academicSessionID,
      term_id: termID,
    }
  );
}

export async function removeClassSubjectOffering(
  classID: string,
  subjectID: string,
  academicSessionID: string,
  termID: string
): Promise<ClassSubjectOfferingResponse> {
  const query = new URLSearchParams({
    academic_session_id: academicSessionID,
    term_id: termID,
  });

  return resultRequest<ClassSubjectOfferingResponse>(
    `admin/classes/${classID}/subjects/${subjectID}?${query.toString()}`,
    "DELETE"
  );
}
