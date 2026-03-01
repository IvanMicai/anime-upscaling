import type {
  Job,
  FilesResponse,
  CreateJobRequest,
  CreateJobResponse,
  CancelJobResponse,
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

export function getFiles(dir: string = "input", refresh = false): Promise<FilesResponse> {
  const params = new URLSearchParams({ dir });
  if (refresh) params.set("refresh", "true");
  return fetchJSON<FilesResponse>(`/api/files?${params}`);
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
