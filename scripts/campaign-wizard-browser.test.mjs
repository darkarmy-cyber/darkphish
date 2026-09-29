import test from "node:test"
import { browserFixture, read, report, script, style } from "./browser-fixture.mjs"

test("campaign wizard enforces review, preserves licensing and safely renders real Select2 selections", () => {
  const template = read("templates/campaigns.html").split('{{end}} {{define "scripts"}}')[0]
    .replace(/{{[\s\S]*?}}/g, "")
  browserFixture(`<!doctype html><html><head>${style("static/css/dist/darkphish.css")}</head>
    <body class="classic-ui">${template}
    ${script("static/js/src/vendor/jquery.js")}${script("static/js/src/vendor/select2.min.js")}
    <script>var launches = 0; window.launch = function () {
      if (!$("#launchButton").prop("disabled")) launches++;
    };</script>${script("static/js/src/app/campaign_wizard.js")}
    ${report(`
      const visible = id => getComputedStyle(document.getElementById(id)).display !== "none";
      const next = () => $("#campaignWizardNext").trigger("click");
      $("#modal").addClass("in").show().trigger("shown.bs.modal");
      check(!visible("launchButton"), "Launch must be visually hidden before review");
      check(!visible("campaignWizardBack"), "Back must be hidden at the first step");
      window.launch(); check(launches === 0, "Early launch reached the existing handler");
      next(); check($("#campaignWizardSteps li.active").attr("data-step") === "1", "Empty basics advanced");
      const payload = '<img src=x onerror="window.executed=true">';
      $("#name").val(payload); $("#url").val("https://simulation.example.test");
      for (const id of ["template", "page", "profile", "users"]) {
        $("#" + id).append(new Option(payload, "fixture", true, true)).select2();
      }
      next(); check($("#campaignWizardSteps li.active").attr("data-step") === "2", "Valid basics did not advance");
      $("#credential_min_length").val(129); next();
      check($("#campaignWizardSteps li.active").attr("data-step") === "2", "Invalid policy advanced");
      $("#credential_min_length").val(12); next();
      check(visible("profile") && !visible("name"), "Delivery fields have wrong visibility");
      $("#launch_date").val("September 29th 2026, 6:00 pm"); next();
      check(visible("launchButton") && !visible("campaignWizardNext"), "Review controls have wrong visibility");
      check($("#reviewName").text() === payload && $("#reviewGroups").text() === payload, "Review changed selection text");
      check(!$("#campaignReview img").length && !window.executed, "Review interpreted untrusted HTML");
      check($("#launchButton").prop("disabled"), "Wizard removed licensing disablement");
      window.launch(); check(launches === 0, "Disabled license allowed launch");
      $("#launchButton").prop("disabled", false); window.launch();
      check(launches === 1, "Reviewed launch did not delegate to existing confirmation");
      $("#campaignWizardBack").trigger("click"); window.launch();
      check(launches === 1, "Returning from review allowed launch");
      $("#modal").trigger("hidden.bs.modal").trigger("shown.bs.modal");
      check($("#campaignWizardSteps li.active").attr("data-step") === "1", "Reopen did not reset wizard");
    `)}</body></html>`)
})
