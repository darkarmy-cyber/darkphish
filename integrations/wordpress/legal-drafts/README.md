# Draft service terms and licensing privacy notice

These English documents use the proposed details supplied on 2026-09-16:
Dark Army s. r. o., Slovakia, info@darkarmy.sk. The owner subsequently confirmed
that the company does not yet exist and WordPress email is not configured.
The requested IČO placeholder is `XXXX`. The proposed address is Ulica Ivana
Krasku 4257/27, 917 05 Trnava, Slovakia, taken at the owner's request from
[fsociety's FinStat record](https://www.finstat.sk/56664001). This is not evidence
of Dark Army's registered office. No fsociety company number, tax identifier
or register entry was attributed to Dark Army. They are review drafts, not
approved production policies. Do not point the live registration checkbox at
these drafts or use any draft version as the production terms version.
They are deliberately outside `static-site/` and the installable plugin.

## Outstanding information and decisions

- Identify the real contracting provider and data controller before launch.
  Complete company registration/address/entry and applicable contact/disclosure
  information when available. If launching before incorporation, the owner
  must identify the actual existing provider; this draft does not nominate
  fsociety or a natural person automatically.
- Confirm the actual hosting and mail providers, processing locations,
  contractual arrangements and transfer safeguards. SlovakNET is the user's
  stated host, not a verified legal entity or confirmation of data location.
  The owner corrected the mail provider to Websupport and selected the new
  license@darkphish.sk mailbox for licensing; its creation and SMTP delivery
  remain unconfirmed. sales@darkphish.sk already handles the product website
  sales buttons. info@darkarmy.sk remains the general developer contact.
  WordPress hosting on fsociety.sk remains at SlovakNET. Follow
  [the mail setup guide](MAIL_SETUP.sk.md); no live configuration was changed.
- Select a defensible retention schedule and implement deletion/manual review
  for licenses, audit events, mail, server logs and backups. The present code
  automatically cleans only expired verification requests and rate buckets.
  Do not publish invented 30/90-day deletion guarantees.
- Confirm audience and applicable legal bases; assess contractual information,
  local-language requirements and mandatory consumer remedies with qualified
  Slovak counsel. The request is for English content, but that alone does not
  determine what languages or disclosures the law requires.
- Review the provider commitments (free service, renewal/reset assistance,
  suspension review and terms-copy requests). These are proposed terms and
  operational procedures, not facts independently confirmed about the company.

## Code alignment

Service.php: 365-day issuance, one binding, replacement keys preserve expiry
and revocation, administrator renewal/reset. Signer.php: 30-day validity plus
up to 30-day grace bounded by license expiry. Store.php: retained fields and
cleanup limited to requests/limits. The API emails a verification link and
returns a key on explicit verification; emailing the resulting key has not
been implemented. The root LICENSE grants MIT rights, so these drafts govern
the hosted service and do not purport to revoke software rights.

## After completion and review

Move the approved documents into a reviewed deployment package for
`https://www.darkphish.sk/license/terms/` and `/license/privacy/`, with
`/license/legal.css`. Remove draft labels only after filling the missing facts
and settling the operating policies. Add the approved privacy link next to
registration (a notice, not bundled marketing consent). Preserve dated versions
and make a durable copy available. Set the final HTTPS terms URL and version
in the WordPress settings only after the pages are live. No live settings or
production policy have been changed by creating these files.

## Primary references consulted

- [Slovak electronic commerce act, current portal](https://www.slov-lex.sk/ezbierky/pravne-predpisy/SK/ZZ/2004/22/)
  — check the current consolidated text and applicability, including provider
  identification and electronic contracting information. The static 2022 text
  was readable; current portal rendering was unavailable to the web reader.
- [GDPR, official EUR-Lex text](https://eur-lex.europa.eu/legal-content/EN/TXT/?uri=CELEX%3A32016R0679)
  — Articles 5, 6, 13 and data-subject rights. Search excerpts were available;
  direct full-text access was challenged, so this is not a full current-law audit.
- [Cloudflare Turnstile Privacy Addendum](https://www.cloudflare.com/turnstile-privacy-policy/)
  — provider-described signal processing and roles; inspected 2026-09-16.
- [Slovak data protection authority](https://www.dataprotection.gov.sk/en/contact/)
  — official supervisory authority contact.

The documents are a technical and drafting aid, not a legal compliance
certification. Final review must cover the actual business and deployment.

## Community terms revision 2026-09-16-draft.2

The owner reconfirmed Dark Army s. r. o. as the intended provider, despite
incorporation remaining pending. Keep that identity as planned; do not substitute
fsociety or describe the company as already incorporated. The revised English
Community License Terms include a plain-language summary, contents navigation,
organizational authority, no automatic billing, technical requirements, key
replacement, renewal, offline limits, ending access and independent MIT rights.
The privacy notice remains draft.1, with unresolved retention and processor facts.
No draft URLs or terms version were saved to production.

See [DOKONCENIE.sk.md](DOKONCENIE.sk.md) for the concrete publication steps.

Sources checked again on 2026-09-16: [Slovak consumer protection act](https://www.slov-lex.sk/ezbierky/pravne-predpisy/SK/ZZ/2024/108/)
and [GDPR Article 13, official text](https://eur-lex.europa.eu/eli/reg/2016/679/). The current Slov-Lex e-commerce
portal and direct EUR-Lex page required JavaScript; official indexed excerpts
were available. These checks are not a complete legal review.
