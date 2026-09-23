// Claude OAuth usage bridge. Utilization is already a percentage, not a fraction.
function parse(body, ctx) {
  if (ctx.httpStatus < 200 || ctx.httpStatus >= 300 || !body || body.error) {
    return {
      status: ctx.httpStatus === 401 || ctx.httpStatus === 403 ? "expired" : "error",
      tiers: [],
      error: body && body.error ? String(body.error) : "Claude query failed (HTTP " + ctx.httpStatus + ")"
    };
  }
  var tiers = [];
  var seen = {};
  function add(name, label, percent, reset) {
    if (seen[name] || typeof percent !== "number" || !isFinite(percent) || percent < 0) return;
    var date = typeof reset === "string" ? new Date(reset) : null;
    tiers.push({
      name: name,
      label: label,
      utilization: percent,
      resetsAt: date && !isNaN(date.getTime()) ? date.toISOString() : ""
    });
    seen[name] = true;
  }
  var keys = Object.keys(body).sort();
  var recognized = false;
  for (var i = 0; i < keys.length; i++) {
    var key = keys[i];
    if (key !== "five_hour" && key !== "seven_day" && key.indexOf("seven_day_") !== 0) continue;
    recognized = true;
    var window = body[key];
    if (!window) continue;
    add(key, key === "seven_day" ? "weekly" : key.replace("seven_day_", "weekly · "), window.utilization, window.resets_at);
  }
  // Newer responses may expose model-scoped limits as an array.
  if (Array.isArray(body.limits)) {
    recognized = true;
    for (var j = 0; j < body.limits.length; j++) {
      var limit = body.limits[j];
      if (!limit || limit.is_active === false) continue;
      var model = limit.scope && limit.scope.model;
      var modelID = model && (model.id || model.display_name);
      var kind = limit.kind || limit.group;
      if (!kind) continue;
      var name = "limit:" + kind + (modelID ? ":" + modelID : "");
      var label = (model && (model.display_name || model.id) ? (model.display_name || model.id) + " · " : "") + (limit.group || kind);
      // Both formats can be present in the same response. Keep stable series IDs.
      if (kind === "session" && !modelID) {
        name = "five_hour";
        label = "five_hour";
      } else if (kind === "weekly_all" && !modelID) {
        name = "seven_day";
        label = "weekly";
      } else if (kind === "weekly_scoped" && modelID) {
        var modelKey = String(model.display_name || modelID).toLowerCase();
        if (modelKey === "sonnet" || modelKey === "opus") name = "seven_day_" + modelKey;
      }
      add(name, label, limit.percent, limit.resets_at);
    }
  }
  if (body.extra_usage && body.extra_usage.is_enabled === true) {
    recognized = true;
    add("extra_usage", "Extra usage", body.extra_usage.utilization, null);
  }
  if (!recognized) return { status: "error", tiers: [], error: "Missing Claude usage windows" };
  return { status: "ok", tiers: tiers };
}
