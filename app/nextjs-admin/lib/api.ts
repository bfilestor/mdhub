import { getApiConfig } from "@/lib/env";

export type FileItem = {
  uuid: string;
  title: string;
  fileName: string;
  type: string;
  syncStatus: string;
};

export type FileDetail = {
  uuid: string;
  title: string;
  fileName: string;
  relativePath: string;
  contentType: string;
  syncStatus: string;
  contentPreview?: string;
};

export async function fetchFiles(params: {
  type?: string;
  syncStatus?: string;
}): Promise<{ items: FileItem[]; error?: string }> {
  const cfg = getApiConfig();
  if ("error" in cfg) {
    return { items: [], error: cfg.error };
  }

  const query = new URLSearchParams();
  if (params.type) query.set("type", params.type);
  if (params.syncStatus) query.set("syncStatus", params.syncStatus);

  const res = await fetch(`${cfg.baseUrl}/api/v1/files?${query.toString()}`, {
    headers: cfg.token ? { Authorization: `Bearer ${cfg.token}` } : undefined,
    cache: "no-store",
  });

  if (!res.ok) {
    return { items: [], error: `API ${res.status}` };
  }

  const data = (await res.json()) as { items?: FileItem[] };
  return { items: data.items ?? [] };
}

export async function fetchFileDetail(uuid: string): Promise<{
  item?: FileDetail;
  error?: string;
}> {
  const cfg = getApiConfig();
  if ("error" in cfg) {
    return { error: cfg.error };
  }

  const res = await fetch(`${cfg.baseUrl}/api/v1/files/${uuid}`, {
    headers: cfg.token ? { Authorization: `Bearer ${cfg.token}` } : undefined,
    cache: "no-store",
  });

  if (res.status === 404) {
    return { error: "文件不存在或已删除" };
  }
  if (!res.ok) {
    return { error: `API ${res.status}` };
  }

  const item = (await res.json()) as FileDetail;
  return { item };
}

export async function deleteFile(uuid: string): Promise<{ error?: string }> {
  const cfg = getApiConfig();
  if ("error" in cfg) {
    return { error: cfg.error };
  }

  const res = await fetch(`${cfg.baseUrl}/api/v1/files/${uuid}`, {
    method: "DELETE",
    headers: cfg.token ? { Authorization: `Bearer ${cfg.token}` } : undefined,
    cache: "no-store",
  });

  if (!res.ok) {
    return { error: `删除失败: API ${res.status}` };
  }
  return {};
}
