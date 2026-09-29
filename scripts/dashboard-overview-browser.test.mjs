import test from "node:test"
import { browserFixture, read, report, script, style } from "./browser-fixture.mjs"

for (const width of [600, 900, 1440]) {
  test(`dashboard renders current summary metrics safely at ${width}px`, () => {
    const template = read("templates/dashboard.html").split('{{end}} {{define "scripts"}}')[0]
      .replace(/{{[\s\S]*?}}/g, "")
    browserFixture(`<!doctype html><html><head><meta name="viewport" content="width=device-width">
      ${style("static/css/dist/darkphish.css")}</head><body class="classic-ui">${template}
      ${script("static/js/src/vendor/jquery.js")}<script>window.campaigns = [];</script>
      ${script("static/js/src/app/dashboard_overview.js")}${report(`
        $("#dashboard").show(); $("#loading, #emptyMessage").hide();
        const refresh = () => $(document).trigger("ajaxComplete");
        const text = id => document.getElementById(id).textContent;
        const campaign = { id: 1, status: "In progress", created_date: "2026-09-29",
          name: '<img src=x onerror="window.executed=true">',
          stats: { total: 10, sent: 8, opened: 4, clicked: 2, email_reported: 1 } };
        window.campaigns = [campaign]; refresh();
        check(text("kpiRecipients") === "10" && text("kpiSentRate") === "80%", "Initial totals are wrong");
        check(text("kpiOpenRate") === "40%" && text("kpiClickRate") === "20%" && text("kpiReportRate") === "10%", "Metric semantics changed");
        campaign.stats.opened = 7; refresh();
        check(text("kpiOpenRate") === "70%", "Stats-only refresh left stale KPI data");
        window.campaigns.push({ id: 2, stats: { total: 30, sent: 12, opened: 3, clicked: 2, email_reported: 3 } }); refresh();
        check(text("kpiCampaigns") === "2" && text("kpiSentRate") === "50%" && text("kpiOpenRate") === "25%", "Aggregate rates must weight by recipients");
        for (const card of document.querySelectorAll("#analyticsKpis .well")) {
          const bounds = card.getBoundingClientRect();
          check(bounds.width > 0 && bounds.left >= 0 && bounds.right <= innerWidth + 1, "KPI card escapes viewport");
          check(card.scrollWidth <= card.clientWidth + 1, "KPI card clips its text");
        }
        window.campaigns = []; refresh();
        check(text("kpiRecipients") === "0" && text("kpiCampaigns") === "0" && text("kpiOpenRate") === "0%", "Empty results retained old metrics");
        window.campaigns = [{ id: 3, stats: { total: 0 } }]; refresh();
        check(text("kpiSentRate") === "0%", "Zero recipients produced an invalid rate");
        window.campaigns = [{ id: 4, stats: { total: '<img src=x onerror="window.executed=true">' } }]; refresh();
        check(!document.querySelector("#analyticsKpis img") && !window.executed, "Metrics interpreted HTML");
      `)}</body></html>`, { width })
  })
}
