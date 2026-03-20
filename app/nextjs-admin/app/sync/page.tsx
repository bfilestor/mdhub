import { getApiConfig } from "@/lib/env";

export default function SyncPage() {
  const cfg = getApiConfig();

  return (
    <section className="card">
      <h2>/sync</h2>
      {"error" in cfg ? (
        <p className="muted">{cfg.error}</p>
      ) : (
        <p className="muted">同步状态页占位，API: {cfg.baseUrl}</p>
      )}
    </section>
  );
}
