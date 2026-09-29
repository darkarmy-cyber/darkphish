import test from "node:test"
import { browserFixture, read, report, script, style } from "./browser-fixture.mjs"

const themeReport = body => report(body).replace("$(function ()", 'window.addEventListener("load", function ()')

const startup = read("templates/base.html").match(/<script>\s*(try \{[\s\S]*?catch \(e\) \{\})\s*<\/script>/)[1]
const fixture = `<button class="btn btn-default darkphish-theme-toggle" id="themeToggle" aria-pressed="false"><i class="fa fa-moon-o"></i><span class="sr-only">Toggle dark theme</span></button>
  <main class="main"><h1>Dashboard</h1><div class="well" id="surface">Operational overview</div>
  <p class="help-block" id="help">Review your settings</p><input class="form-control" id="field" placeholder="Campaign name">
  <button class="btn btn-primary" id="action">Save</button><button class="btn btn-primary test-hover" id="hover">Save</button>
  <button class="btn btn-warning" id="warning">Review</button><span class="label label-success" id="status">Completed</span>
  <div class="well"><a id="plainLink" href="#">Documentation</a><a id="hoverLink" class="test-hover" href="#">Documentation</a></div>
  <div class="alert alert-warning"><a class="alert-link" id="licenseLink" href="#">Activate license</a></div>
  <div class="modal" style="position:static;display:block"><div class="modal-content"><div class="modal-header"><button type="button" class="close" id="modalClose">×</button><h4>Confirm</h4></div></div></div>
  <table id="sortable" class="table"><thead><tr><th>Name</th><th>Count</th></tr></thead><tbody><tr><td>Example</td><td>1</td></tr></tbody></table>
  <div id="chart" style="height:200px"></div>
  <table class="table table-striped"><tbody><tr><td id="striped">Campaign</td></tr><tr><td>Another campaign</td></tr></tbody></table>
  <select id="selection"><option selected>Selected group</option></select>
  <div class="cke_chrome"><div class="cke_top" id="toolbar"><span class="cke_button_label" id="editorLabel">Source</span></div><div class="cke_contents" id="canvas">Authored content</div></div></main>`

for (const width of [600, 1440]) {
  test(`dark theme has readable real styles and a working toggle at ${width}px`, () => {
    const hoverStyles = (read("static/css/dark-theme.css").match(/[^{}]*:hover[^{}]*\{[^{}]*\}/g) || []).join("\n").replaceAll(":hover", ".test-hover")
    browserFixture(`<!doctype html><html><head>${style("static/css/dist/darkphish.css")}
      ${style("static/js/src/vendor/ckeditor/skins/moono-lisa/editor.css")}
      <style>${hoverStyles} #status { transition: none !important; }</style>
      <script>localStorage.setItem("darkphish-theme", "dark"); ${startup}</script></head>
      <body class="classic-ui">${fixture}${script("static/js/dist/vendor.min.js")}${script("static/js/src/app/theme.js")}
      ${themeReport(`
        const luminance = color => {
          const values = color.match(/[\\d.]+/g).slice(0, 3).map(Number).map(x => x / 255).map(x => x <= .04045 ? x / 12.92 : ((x + .055) / 1.055) ** 2.4);
          return values[0] * .2126 + values[1] * .7152 + values[2] * .0722;
        };
        const background = element => {
          const value = getComputedStyle(element).backgroundColor;
          return value === "rgba(0, 0, 0, 0)" ? background(element.parentElement) : value;
        };
        const readable = (element, name) => {
          const fg = luminance(getComputedStyle(element).color), bg = luminance(background(element));
          const ratio = (Math.max(fg, bg) + .05) / (Math.min(fg, bg) + .05);
          check(ratio >= 4.5, name + " contrast is " + ratio.toFixed(2));
        };
        check(document.documentElement.dataset.theme === "dark", "Saved theme did not apply");
        $("#selection").select2({ theme: "bootstrap" });
        for (const id of ["surface", "help", "field", "action", "hover", "warning", "status", "striped", "editorLabel", "plainLink", "hoverLink", "licenseLink", "modalClose"]) readable(document.getElementById(id), id);
        check(getComputedStyle(document.getElementById("modalClose")).opacity === "1", "Modal close icon is translucent");
        readable(document.querySelector(".select2-selection__rendered"), "Select2 selected text");
        $("#selection").select2("open");
        readable(document.querySelector(".select2-search__field"), "Select2 search");
        $("#selection").select2("close");
        for (const kind of ["success", "warning", "danger", "info", "primary", "clicked", "default"]) {
          const label = document.getElementById("status"); label.className = "label label-" + kind;
          readable(label, "Status " + kind);
        }
        $("#sortable").DataTable();
        const sorting = getComputedStyle(document.querySelector("#sortable th.sorting"), "::after");
        check(sorting.color === "rgb(183, 196, 187)" && sorting.opacity === "1", "Sort affordance is invisible");
        const chart = Highcharts.chart("chart", { chart: { animation: false }, title: { text: "Analytics" }, series: [{ data: [1, 2], color: "#1abc9c" }] });
        check(getComputedStyle(document.querySelector("#chart .highcharts-background")).fill === "rgb(24, 32, 29)", "Chart canvas remains light");
        check(getComputedStyle(document.querySelector("#chart .highcharts-title")).fill === "rgb(232, 239, 233)", "Chart title is unreadable");
        check(chart.series[0].color === "#1abc9c", "Theme changed metric series meaning");
        check(getComputedStyle(document.getElementById("canvas")).backgroundColor === "rgb(255, 255, 255)", "Editor canvas must preserve authored colors");
        const button = document.getElementById("themeToggle");
        button.focus(); check(getComputedStyle(button).outlineStyle !== "none", "Keyboard focus is invisible");
        check(button.getAttribute("aria-pressed") === "true", "Toggle state was not announced");
        button.click(); check(!document.documentElement.hasAttribute("data-theme") && localStorage.getItem("darkphish-theme") === "light", "Light choice was not persisted");
        button.click(); check(document.documentElement.dataset.theme === "dark", "Dark toggle failed");
        check(getComputedStyle(document.getElementById("action")).transitionDuration.split(",").every(x => parseFloat(x) <= .001), "Reduced motion is ignored");
        Swal.fire({ title: "Confirm action", text: "Review the selected action", showCloseButton: true, animation: false });
        for (const selector of [".swal2-title", ".swal2-content", ".swal2-close"]) readable(document.querySelector(selector), selector);
        Swal.close();
      `)}</body></html>`, { width, flags: ["--force-prefers-reduced-motion"] })
  })
}

test("theme remains usable when local storage is unavailable", () => {
  browserFixture(`<!doctype html><html><head><script>
    Object.defineProperty(window, "localStorage", { get() { throw new Error("Storage unavailable"); } });
    ${startup}</script></head><body>${fixture}${script("static/js/src/vendor/jquery.js")}
    ${script("static/js/src/app/theme.js")}${themeReport(`
      document.getElementById("themeToggle").click();
      check(document.documentElement.dataset.theme === "dark", "Storage failure prevented theme switching");
    `)}</body></html>`)
})
