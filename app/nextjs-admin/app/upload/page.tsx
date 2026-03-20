import { getApiConfig } from "@/lib/env";

export default function UploadPage() {
  const cfg = getApiConfig();

  return (
    <section className="card">
      <h2>/upload</h2>
      {"error" in cfg ? (
        <p className="muted">{cfg.error}</p>
      ) : (
        <p className="muted">上传页占位，API: {cfg.baseUrl}</p>
      )}
    </section>
  );
}
