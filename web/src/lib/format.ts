const validDate = (at: unknown) => {
  const value = typeof at === "number" ? at : Number(at);
  if (!Number.isFinite(value)) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
};

export const clock = (at: unknown) =>
  validDate(at)?.toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }) ?? "Unknown time";
export const dateTime = (at: unknown) =>
  validDate(at)?.toLocaleString([], {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }) ?? "Unknown time";
export const pct = (value: number) => Number(value.toFixed(1)).toString();
