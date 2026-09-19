// Codex quota bridge: GET /api/usage, no authentication headers required.
function parse(body, ctx) {
  if (ctx.httpStatus < 200 || ctx.httpStatus >= 300 || !body || body.error) {
    return {
      status: ctx.httpStatus === 401 || ctx.httpStatus === 403 ? "expired" : "error",
      tiers: [],
      error: body && body.error ? String(body.error) : "Codex query failed (HTTP " + ctx.httpStatus + ")"
    };
  }

  var buckets = body.rateLimitsByLimitId;
  var ids = buckets && typeof buckets === "object" ? Object.keys(buckets).sort() : [];
  if (ids.length === 0 && body.rateLimits) {
    buckets = { codex: body.rateLimits };
    ids = ["codex"];
  }
  var tiers = [];
  var plan = "";
  for (var i = 0; i < ids.length; i++) {
    var bucket = buckets[ids[i]];
    if (!bucket) continue;
    if (bucket.planType) plan = bucket.planType;
    var windows = ["primary", "secondary"];
    for (var j = 0; j < windows.length; j++) {
      var window = bucket[windows[j]];
      if (!window || typeof window.usedPercent !== "number" || !isFinite(window.usedPercent)) continue;
      var minutes = window.windowDurationMins;
      var label = minutes === 300 ? "five_hour" : minutes === 10080 ? "weekly" :
        (typeof minutes === "number" && minutes > 0 ? minutes + "_minutes" : windows[j]);
      var date = typeof window.resetsAt === "number" ? new Date(window.resetsAt * 1000) : null;
      tiers.push({
        name: ids[i] + ":" + windows[j],
        label: (ids.length > 1 ? (bucket.limitName || ids[i]) + " · " : "") + label,
        utilization: window.usedPercent,
        resetsAt: date && !isNaN(date.getTime()) ? date.toISOString() : ""
      });
    }
  }
  if (ids.length === 0) return { status: "error", tiers: [], error: "Missing Codex rate limits" };
  return { status: "ok", message: plan, tiers: tiers };
}
