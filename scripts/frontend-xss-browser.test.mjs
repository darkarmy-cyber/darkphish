import assert from "node:assert/strict"
import { execFileSync } from "node:child_process"
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { dirname, join, resolve } from "node:path"
import test from "node:test"
import { fileURLToPath } from "node:url"

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const readSource = (path) => readFileSync(join(root, path), "utf8")

test("stored metadata and campaign names remain text in a real browser", () => {
    const campaignResults = readSource("static/js/src/app/campaign_results.js")
    const sendingProfiles = readSource("static/js/src/app/sending_profiles.js")
    const templates = readSource("static/js/src/app/templates.js")
    const campaigns = readSource("static/js/src/app/campaigns.js")

    assert.match(campaignResults, /escapeHtml\(details\.error\)/)
    assert.match(sendingProfiles, /escapeHtml\(profile\.interface_type\)/)
    assert.match(templates, /escapeHtml\(file\.type \|\| "application\/octet-stream"\)/)
    assert.match(campaigns, /confirmButtonText:\s*"Delete Campaign"/)
    assert.doesNotMatch(campaigns, /confirmButtonText:\s*"Delete "\s*\+/)

    const chrome = [
        "/usr/bin/google-chrome",
        "/usr/bin/chromium",
        "/usr/bin/chromium-browser",
    ].find(existsSync)
    assert.ok(chrome, "Chrome or Chromium is required for the DOM security regression test")

    const encodedCampaigns = Buffer.from(campaigns).toString("base64")
    const payload = '<img src=x onerror="document.body.dataset.executed=\'yes\'">'
    const html = `<!doctype html>
<html>
<body data-executed="no">
<div id="smtp-error"></div>
<div id="interface-type"></div>
<div id="attachment-type"></div>
<div id="dialog"></div>
<script>
const payload = ${JSON.stringify(payload)}
function escapeHtml(text) {
    const node = document.createElement("div")
    node.textContent = text
    return node.innerHTML
}
for (const id of ["smtp-error", "interface-type", "attachment-type"]) {
    document.getElementById(id).innerHTML = escapeHtml(payload)
}
function jqueryStub() {
    return { ready() {} }
}
jqueryStub.fn = { select2: { defaults: { set() {} } } }
window.$ = jqueryStub
window.Swal = {
    fire(options) {
        const button = document.createElement("button")
        button.id = "confirm"
        button.innerHTML = options.confirmButtonText
        document.getElementById("dialog").appendChild(button)
        return { then() {} }
    },
}
eval(atob("${encodedCampaigns}"))
campaigns = [{ id: 1, name: payload }]
deleteCampaign(0)
document.body.dataset.dialogText = document.getElementById("confirm").textContent
document.body.dataset.renderedText = document.getElementById("smtp-error").textContent
</script>
</body>
</html>`

    const directory = mkdtempSync(join(tmpdir(), "darkphish-browser-xss-"))
    try {
        const fixture = join(directory, "fixture.html")
        writeFileSync(fixture, html)
        const output = execFileSync(chrome, [
            "--headless=new",
            "--no-sandbox",
            "--disable-gpu",
            "--disable-dev-shm-usage",
            "--dump-dom",
            `file://${fixture}`,
        ], { encoding: "utf8", timeout: 30000 })
        assert.match(output, /data-executed="no"/)
        assert.match(output, /data-dialog-text="Delete Campaign"/)
        assert.ok(output.includes("data-rendered-text="))
        assert.ok(output.includes("&lt;img"))
        assert.doesNotMatch(output, /<img src="x"/)
    } finally {
        rmSync(directory, { recursive: true, force: true })
    }
})
