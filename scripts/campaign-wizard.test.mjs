import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

const read = path => readFileSync(new URL("../" + path, import.meta.url), "utf8")

test("v0.22 campaign modal exposes four explicit setup stages", () => {
  const html = read("templates/campaigns.html")
  for (const label of ["Basics", "Credential policy", "Delivery", "Review"]) {
    assert.match(html, new RegExp(">" + label + "<"))
  }
  assert.match(html, /id="campaignWizardBack"/)
  assert.match(html, /id="campaignWizardNext"/)
  assert.match(html, /id="campaignReview"/)
  assert.match(html, /campaign_wizard\.js\?v=\{\{\.Version\}\}/)
})

test("campaign wizard keeps the existing launch control and licensing gate", () => {
  const html = read("templates/campaigns.html")
  assert.match(html, /id="launchButton"[^>]+onclick="launch\(\)"/)
  assert.match(html, /not \.SendingAllowed/)
  const source = read("static/js/src/app/campaign_wizard.js")
  assert.match(source, /var originalLaunch = window\.launch/)
  assert.match(source, /return originalLaunch\.apply\(this, arguments\)/)
  assert.match(source, /Complete the campaign review before launching/)
})

test("campaign review renders operator data as text rather than HTML", () => {
  const source = read("static/js/src/app/campaign_wizard.js")
  for (const id of [
    "reviewName","reviewTemplate","reviewPage","reviewUrl","reviewCredentialMode",
    "reviewCredentialRetention","reviewCredentialLength","reviewCredentialCharacters","reviewCredentialPatterns",
    "reviewProfile","reviewGroups","reviewLaunchDate","reviewSendByDate"
  ]) {
    assert.match(source, new RegExp("\\$\\(\"#" + id + "\"\\)\\.text\\("))
  }
  assert.doesNotMatch(source, /review(?:Name|Template|Page|Url|CredentialMode|Profile|Groups|LaunchDate|SendByDate)[^\n]*\.html\(/)
})

test("wizard validates required selections before the final review", () => {
  const source = read("static/js/src/app/campaign_wizard.js")
  assert.match(source, /Enter a campaign name before continuing/)
  assert.match(source, /Select an email template before continuing/)
  assert.match(source, /Select a landing page before continuing/)
  assert.match(source, /Select a sending profile before continuing/)
  assert.match(source, /Select at least one target group before continuing/)
  assert.match(source, /Credential length policy is invalid/)
})


test("wizard tolerates slow Select2 initialization and ships in the runtime image", () => {
  const source = read("static/js/src/app/campaign_wizard.js")
  assert.match(source, /hasClass\("select2-hidden-accessible"\)/)
  assert.match(source, /function select2Data/)
  const html = read("templates/campaigns.html")
  assert.match(html, /for="launch_date">Launch Date/)
  const docker = read("Dockerfile")
  assert.match(docker, /campaign_wizard\.js \.\/static\/js\/src\/app\/campaign_wizard\.js/)
})
