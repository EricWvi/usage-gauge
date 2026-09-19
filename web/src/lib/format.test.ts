import { test } from "node:test";
import assert from "node:assert/strict";
import { clock, dateTime } from "./format.ts";

test("date formatting never renders Invalid Date", () => {
  assert.equal(dateTime("5-hour window"), "Unknown time");
  assert.equal(clock(undefined), "Unknown time");
  assert.notEqual(dateTime(1_800_000_000_000), "Unknown time");
});
