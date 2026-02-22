import type {
  Job,
  FilesResponse,
  CreateJobRequest,
  CreateJobResponse,
  CancelJobResponse,
  Source,
  SourceFile,
} from "./types";

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || res.statusText);
  }
  return res.json();
}

export function getJobs(): Promise<Job[]> {
  return fetchJSON<Job[]>("/api/jobs");
}

export function getJob(id: string): Promise<Job> {
  return fetchJSON<Job>(`/api/jobs/${id}`);
}

export function getFiles(dir: string = "input"): Promise<FilesResponse> {
  return fetchJSON<FilesResponse>(`/api/files?dir=${dir}`);
}

export function createJob(req: CreateJobRequest): Promise<CreateJobResponse> {
  return fetchJSON<CreateJobResponse>("/api/jobs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function cancelJob(id: string): Promise<CancelJobResponse> {
  return fetchJSON<CancelJobResponse>(`/api/jobs/${id}/cancel`, {
    method: "POST",
  });
}

export function getSources(): Promise<Source[]> {
  return fetchJSON<Source[]>("/api/sources");
}

export function createSource(req: { name: string; path: string }): Promise<Source> {
  return fetchJSON<Source>("/api/sources", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function deleteSource(id: string): Promise<{ deleted: string }> {
  return fetchJSON<{ deleted: string }>(`/api/sources/${id}`, {
    method: "DELETE",
  });
}

export function getSourceFiles(id: string): Promise<{ files: SourceFile[] }> {
  return fetchJSON<{ files: SourceFile[] }>(`/api/sources/${id}/files`);
}

export function importFiles(id: string, files: string[]): Promise<{ copied: number }> {
  return fetchJSON<{ copied: number }>(`/api/sources/${id}/import`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ files }),
  });
}

export function exportFiles(
  id: string,
  files: string[],
  from: string
): Promise<{ copied: number }> {
  return fetchJSON<{ copied: number }>(`/api/sources/${id}/export`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ files, from }),
  });
}
