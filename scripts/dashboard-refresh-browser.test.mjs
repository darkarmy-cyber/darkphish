import test from "node:test"
import { browserFixture, read, report, script, style } from "./browser-fixture.mjs"

test("refresh fetches one new summary and updates cards, table and charts together", () => {
  const template = read("templates/dashboard.html").split('{{end}} {{define "scripts"}}')[0].replace(/{{[\s\S]*?}}/g, "")
  browserFixture(`<!doctype html><html><head>${style("static/css/dist/darkphish.css")}</head>
    <body class="classic-ui"><div id="flashes"></div>${template}${script("static/js/dist/vendor.min.js")}
    <script>
      var requests = [], errors = [];
      var api = { campaigns: { summary: function () {
        var pending = $.Deferred(); requests.push(pending);
        var promise = pending.promise(); promise.success = promise.done; promise.error = promise.fail;
        return promise;
      } } };
      function escapeHtml(text) { return $("<div>").text(text).html(); }
      function errorFlash(message) { errors.push(message); }
    </script>${script("static/js/dist/app/dashboard.min.js")}${script("static/js/src/app/dashboard_overview.js")}
    ${report(`
      const snapshot = opened => ({ campaigns: [{ id: 1, name: "Authorized simulation", status: "In progress",
        created_date: "2026-09-29T12:00:00Z", launch_date: "2026-09-29T12:00:00Z",
        stats: { total: 10, sent: 8, opened, clicked: 2, email_reported: 1, submitted_data: 0, error: 0 } }] });
      check(requests.length === 1, "Initial load did not request a summary");
      requests[0].resolve(snapshot(4));
      check($("#kpiOpenRate").text() === "40%", "Initial KPI did not render");
      $("#analyticsRefresh").trigger("click");
      check(requests.length === 2, "Refresh reused the old snapshot");
      refreshDashboard(); check(requests.length === 2, "Overlapping requests were allowed");
      check($("#analyticsRefresh").prop("disabled"), "Pending refresh did not disable the button");
      requests[1].resolve(snapshot(7));
      check($("#kpiOpenRate").text() === "70%", "Fetched KPI did not update");
      const table = $("#campaignTable").DataTable();
      check(table.rows().count() === 1 && table.row(0).data()[3] === 7, "Table was duplicated or left stale");
      const chart = Highcharts.charts.find(item => item && item.renderTo.id === "opened_chart");
      check(chart.series[0].data[0].count === 7, "Chart did not use the new summary");
      $("#analyticsRefresh").trigger("click"); requests[2].resolve({ campaigns: [] });
      check($("#kpiCampaigns").text() === "0" && table.rows().count() === 0, "Empty response left old data");
      check($("#emptyMessage").is(":visible") && !$("#dashboard").is(":visible"), "Empty state is wrong");
      $("#analyticsRefresh").trigger("click"); requests[3].reject({ status: 403 });
      check(errors.length === 1 && !$("#analyticsRefresh").prop("disabled"), "Failed refresh did not recover");
      check(!$("#dashboard").is(":visible"), "Failed authorization retained displayed results");
      $("#analyticsRefresh").trigger("click"); requests[4].resolve(snapshot(5));
      check($("#kpiOpenRate").text() === "50%" && $("#dashboard").is(":visible"), "Retry did not recover");
    `)}</body></html>`)
})
