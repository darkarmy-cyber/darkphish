import { readFileSync } from "node:fs"

// Historical recovery is available only through an explicit main-branch
// workflow dispatch. Scheduled recovery continues to use the current VERSION.
export function recoveryVersion(env = process.env, current = readFileSync("VERSION", "utf8").trim()) {
  const requested = env.RECOVERY_VERSION_OVERRIDE || ""
  if (!requested) return current
  if (requested !== "0.27.2" || current !== "0.27.3" || env.GITHUB_EVENT_NAME !== "workflow_dispatch") {
    throw new Error("historical release recovery override is not authorized for this version or event")
  }
  return requested
}
