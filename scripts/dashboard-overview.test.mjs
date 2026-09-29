import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const read = path => readFileSync(new URL("../" + path, import.meta.url), "utf8")

test("v0.22 dashboard groups analytics into operational sections", () => {
  const html = read("templates/dashboard.html")
  for (const heading of ["Operational Overview", "Engagement Trend", "Aggregate Outcomes", "Recent Campaigns"]) {
    assert.match(html, new RegExp(heading))
  }
  for (const id of ["kpiCampaigns","kpiRecipients","kpiSentRate","kpiOpenRate","kpiClickRate","kpiReportRate"]) {
    assert.match(html, new RegExp('id="' + id + '"'))
  }
  assert.match(html, /dashboard_overview\.js\?v=\{\{\.Version\}\}/)
})

test("dashboard overview preserves metric definitions and renders text-only values", () => {
  const source = read("static/js/src/app/dashboard_overview.js")
  assert.match(source, /stats\.total/)
  assert.match(source, /stats\.sent/)
  assert.match(source, /stats\.opened/)
  assert.match(source, /stats\.clicked/)
  assert.match(source, /stats\.email_reported/)
  assert.match(source, /Math\.round\(\(count \/ total\) \* 100\)/)
  for (const id of ["kpiCampaigns","kpiRecipients","kpiSentRate","kpiOpenRate","kpiClickRate","kpiReportRate"]) {
    assert.match(source, new RegExp('\\$\\("#' + id + '"\\)\\.text\\('))
  }
  assert.doesNotMatch(source, /\.html\(/)
})
