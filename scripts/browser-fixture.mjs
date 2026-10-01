import assert from "node:assert/strict"
import { execFileSync } from "node:child_process"
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { pathToFileURL } from "node:url"

export const read = path => readFileSync(new URL("../" + path, import.meta.url), "utf8")
export const script = path => `<script>${read(path).replace(/<\/script/gi, "<\\/script")}</script>`
export const style = path => `<style>${read(path)}</style>`

// Use an isolated profile and local fixtures; tests never send campaigns or email.
export function browserFixture(html, { width = 1280, flags = [] } = {}) {
  const chrome = [process.env.CHROME_BIN, "/usr/bin/google-chrome", "/usr/bin/chromium",
    "/usr/bin/chromium-browser", "C:/Program Files/Google/Chrome/Application/chrome.exe",
    "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe"].filter(Boolean).find(existsSync)
  assert.ok(chrome, "Chrome or Chromium is required for browser regression coverage")
  const directory = mkdtempSync(join(tmpdir(), "darkphish-ui-"))
  try {
    const file = join(directory, "fixture.html")
    writeFileSync(file, html)
    const output = execFileSync(chrome, ["--headless=new", "--no-sandbox", "--disable-gpu",
      "--disable-dev-shm-usage", "--no-first-run", "--disable-background-networking",
      "--disable-threaded-animation", "--run-all-compositor-stages-before-draw",
      `--user-data-dir=${join(directory, "profile")}`, `--window-size=${width},1000`,
      "--virtual-time-budget=5000", ...flags, "--dump-dom", pathToFileURL(file).href],
    { encoding: "utf8", timeout: 60000, maxBuffer: 8 * 1024 * 1024, windowsHide: true, stdio: ["ignore", "pipe", "pipe"] })
    const result = output.match(/<pre id="test-result">([^<]*)<\/pre>/)
    assert.ok(result, "Browser did not finish the fixture")
    assert.equal(result[1], "PASS", result[1])
    return output
  } finally {
    rmSync(directory, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 })
  }
}

export const report = body => `<script>$(function () {
  const result = document.createElement("pre"); result.id = "test-result";
  const check = (condition, message) => { if (!condition) throw new Error(message); };
  try { ${body}\nresult.textContent = "PASS"; }
  catch (error) { result.textContent = error.message; }
  document.body.appendChild(result);
});</script>`
