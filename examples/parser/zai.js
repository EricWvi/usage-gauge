// Example parser for zai (ZhipuAI coding plan quota).
// Copy to config/parser/zai.js to customize; otherwise the built-in parser
// (same logic) is used automatically.
//
// Replicates cc-switch-cli query_zhipu (coding_plan.rs:184-285).
// ES5.1 syntax (goja does not reliably support arrow functions / template literals).

function parse(body, ctx) {
  var queriedAt = new Date().getTime();
  var s = ctx.httpStatus;

  if (s === 401 || s === 403) {
    return {
      status: "expired",
      message: "Invalid API key",
      tiers: [],
      error: "Authentication failed (HTTP " + s + ")",
      queriedAt: queriedAt
    };
  }

  if (s < 200 || s >= 300) {
    return {
      status: "error",
      tiers: [],
      error: "API error (HTTP " + s + "): " + ctx.rawBody,
      queriedAt: queriedAt
    };
  }

  if (body == null || typeof body !== "object") {
    return {
      status: "error",
      tiers: [],
      error: "Failed to parse response: not a JSON object",
      queriedAt: queriedAt
    };
  }

  if (body.success === false) {
    return {
      status: "error",
      tiers: [],
      error: "API error: " + (body.msg || "Unknown error"),
      queriedAt: queriedAt
    };
  }

  var data = body.data;
  if (data == null) {
    return {
      status: "error",
      tiers: [],
      error: "Missing 'data' field in response",
      queriedAt: queriedAt
    };
  }

  var limits = Array.isArray(data.limits) ? data.limits : [];
  var tiers = [];
  for (var i = 0; i < limits.length; i++) {
    var item = limits[i];
    if (item.type !== "TOKENS_LIMIT") {
      continue;
    }
    tiers.push({
      name: "five_hour",
      utilization: typeof item.percentage === "number" ? item.percentage : 0,
      resetsAt: millisToISO(item.nextResetTime)
    });
  }

  return {
    status: "ok",
    message: typeof data.level === "string" ? data.level : "",
    tiers: tiers,
    queriedAt: queriedAt
  };
}

function millisToISO(ms) {
  if (typeof ms !== "number" || !isFinite(ms)) {
    return "";
  }
  var d = new Date(ms);
  return isNaN(d.getTime()) ? "" : d.toISOString();
}
