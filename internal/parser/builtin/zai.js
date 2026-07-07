// Builtin parser for zai (ZhipuAI coding plan quota).
// Replicates cc-switch-cli query_zhipu (coding_plan.rs:184-285).
//
// The framework has already issued GET <endpoint.url> with the configured
// headers (raw Authorization key, NO Bearer prefix) plus Accept-Language.
// This function only derives a UsageResult from httpStatus + body.
//
// ES5.1 syntax: goja does not reliably support arrow functions / template
// literals, so stick to function/var/string concatenation.

function parse(body, ctx) {
  var queriedAt = new Date().getTime();
  var s = ctx.httpStatus;

  // 1) Credential failure (api key invalid/expired).
  if (s === 401 || s === 403) {
    return {
      status: "expired",
      message: "Invalid API key",
      tiers: [],
      error: "Authentication failed (HTTP " + s + ")",
      queriedAt: queriedAt
    };
  }

  // 2) Other non-2xx: surface the body text.
  if (s < 200 || s >= 300) {
    return {
      status: "error",
      tiers: [],
      error: "API error (HTTP " + s + "): " + ctx.rawBody,
      queriedAt: queriedAt
    };
  }

  // 3) Body must be a JSON object.
  if (body == null || typeof body !== "object") {
    return {
      status: "error",
      tiers: [],
      error: "Failed to parse response: not a JSON object",
      queriedAt: queriedAt
    };
  }

  // 4) Business-level error.
  if (body.success === false) {
    return {
      status: "error",
      tiers: [],
      error: "API error: " + (body.msg || "Unknown error"),
      queriedAt: queriedAt
    };
  }

  // 5) Data envelope is required.
  var data = body.data;
  if (data == null) {
    return {
      status: "error",
      tiers: [],
      error: "Missing 'data' field in response",
      queriedAt: queriedAt
    };
  }

  // 6) Keep only TOKENS_LIMIT entries; map to tiers.
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

  // 7) Plan level (e.g. "GLM Coding Plan ...") as the display message.
  var level = typeof data.level === "string" ? data.level : "";

  return {
    status: "ok",
    message: level,
    tiers: tiers,
    queriedAt: queriedAt
  };
}

// Millisecond epoch -> ISO 8601 string; "" if invalid.
function millisToISO(ms) {
  if (typeof ms !== "number" || !isFinite(ms)) {
    return "";
  }
  var d = new Date(ms);
  return isNaN(d.getTime()) ? "" : d.toISOString();
}
