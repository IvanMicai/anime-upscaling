import type {
  Job,
  FilesResponse,
  CreateJobRequest,
  CreateJobResponse,
  CancelJobResponse,
  DeleteFilesRequest,
  DeleteFilesResponse,
  Pipeline,
  CreatePipelineRequest,
  UpdatePipelineRequest,
  RunPipelineRequest,
  Settings,
  SystemStatus,
} from "./types";

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || res.statusText);
  }
  if (res.status === 204) {
    return undefined as T;
  }
  return res.json();
}

export function getJobs(): Promise<Job[]> {
  return fetchJSON<Job[]>("/api/jobs");
}

export function getJob(id: string): Promise<Job> {
  return fetchJSON<Job>(`/api/jobs/${id}`);
}

export function getFiles(dir: string = "input", path: string = "", refresh = false): Promise<FilesResponse> {
  const params = new URLSearchParams({ dir });
  if (path) params.set("path", path);
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

export function deleteJob(id: string): Promise<void> {
  return fetchJSON<void>(`/api/jobs/${id}`, { method: "DELETE" });
}

export function deleteFiles(req: DeleteFilesRequest): Promise<DeleteFilesResponse> {
  return fetchJSON<DeleteFilesResponse>("/api/files", {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

// Pipeline API

export function getPipelines(): Promise<Pipeline[]> {
  return fetchJSON<Pipeline[]>("/api/pipelines");
}

export function getPipeline(id: string): Promise<Pipeline> {
  return fetchJSON<Pipeline>(`/api/pipelines/${id}`);
}

export function createPipeline(req: CreatePipelineRequest): Promise<Pipeline> {
  return fetchJSON<Pipeline>("/api/pipelines", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function updatePipeline(id: string, req: UpdatePipelineRequest): Promise<Pipeline> {
  return fetchJSON<Pipeline>(`/api/pipelines/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function deletePipeline(id: string): Promise<void> {
  return fetchJSON<void>(`/api/pipelines/${id}`, { method: "DELETE" });
}

export function runPipeline(id: string, req: RunPipelineRequest): Promise<CreateJobResponse> {
  return fetchJSON<CreateJobResponse>(`/api/pipelines/${id}/run`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function getSettings(): Promise<Settings> {
  return fetchJSON<Settings>("/api/settings");
}

export function getSystemStatus(): Promise<SystemStatus> {
  return fetchJSON<SystemStatus>("/api/system");
}

export function updateSettings(req: { streams_per_gpu: number; ffmpeg_streams: number; gpu_vendor?: string }): Promise<Settings> {
  return fetchJSON<Settings>("/api/settings", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export function downloadFile(dir: string, name: string, path: string = ""): void {
  const params = new URLSearchParams({ dir, name });
  if (path) params.set("path", path);
  const a = document.createElement("a");
  a.href = `/api/files/download?${params}`;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
}
