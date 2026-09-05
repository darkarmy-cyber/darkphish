const title = process.argv[2] || ""
const files = (process.argv[3] || "").split("\n").filter(Boolean)
const versions = title.match(/\b\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?\b/g) || []
if (versions.length < 2) throw new Error("cannot classify dependency update versions")
const [from, to] = versions.slice(-2).map((value) => value.split(/[+-]/)[0].split(".").map(Number))
if (to[0] !== from[0]) throw new Error("major dependency updates require manual review")
if (to[0] === from[0] && to[1] === from[1] && to[2] === from[2]) throw new Error("update is not a patch or minor SemVer change")
if (to[0] < from[0] || (to[0] === from[0] && to[1] < from[1]) || (to[0] === from[0] && to[1] === from[1] && to[2] < from[2])) throw new Error("dependency downgrades require manual review")
const sensitive = /(crypto|argon|bcrypt|auth|oauth|jwt|session|csrf|gorm|mysql|sqlite|postgres|pgx|lib\/pq|database|goose|vault|kms|sign|codeql|trivy|sbom|attest)/i
if (sensitive.test(title)) throw new Error("security- or database-sensitive dependency requires manual review")
if (files.some((file) => file.startsWith(".github/workflows/"))) throw new Error("workflow privilege changes require engineering review")
if (files.some((file) => /(^|\/)(auth|internal\/secrets|db\/db_)/.test(file))) throw new Error("sensitive files require manual review")
process.stdout.write("eligible patch/minor dependency update\n")
