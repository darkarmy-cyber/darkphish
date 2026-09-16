# Nastavenie overovacích e-mailov

Stav k 16. 9. 2026: pre licencie je zvolená nová schránka
`license@darkphish.sk` u Websupportu. Jej vytvorenie ani doručenie testu ešte
nie sú potvrdené. Existujúca `sales@darkphish.sk` zostáva pre Start Professional
a Contact Sales. Dark Army vyvíja DarkPhish; `info@darkarmy.sk` zostáva
všeobecným firemným kontaktom. WordPress API na fsociety.sk zostáva u SlovakNETu.
Tento návod nemení živé nastavenia a neposiela správy.

## 1. Vytvor licenčnú schránku

Vo Websupport administrácii vytvor samostatnú schránku `license@darkphish.sk`
a over prístup do nej. SMTP používa prihlasovacie údaje tejto schránky.
Heslo zadáš iba do serverového nastavenia pošty, do chatu ho neposielaj.

## 2. Nastav transport WordPressu na fsociety.sk

Licenčný plugin používa štandardné `wp_mail()`. Ak WordPress už má SMTP plugin,
použi existujúci transport; nezapínaj dva naraz. Ak ho nemá, nainštaluj
**WP Mail SMTP by WPForms** a v **WP Mail SMTP → Settings → General** vyber
**Other SMTP**. Nastavenie sa vykonáva na fsociety.sk, aj keď schránku prevádzkuje
Websupport a verejný registračný formulár je na www.darkphish.sk/license/.

| Nastavenie | Hodnota |
| --- | --- |
| SMTP server | `smtp.m1.websupport.sk` |
| SMTP port | `465` |
| Encryption v WP Mail SMTP | **SSL** (implicitné TLS / SMTPS) |
| Authentication | Zapnuté |
| SMTP Username | `license@darkphish.sk` |
| SMTP Password | Heslo novej schránky, zadané súkromne na serveri |
| From Email | `license@darkphish.sk` |
| From Name | DarkPhish Licensing |

Údaje dodal používateľ a zodpovedajú [dokumentácii Websupportu](https://www.websupport.sk/podpora/kb/postove-protokoly/).
Pre port 465 vyber SSL; voľba TLS/STARTTLS v tomto plugine patrí k portu 587.
Certifikáty musia zostať overované. IMAP `imap.m1.websupport.sk`, port `993`
so SSL/TLS, slúži na čítanie schránky; na odosielanie z WordPressu ho nepotrebuješ.

SMTP plugin ovplyvní poštu celého WordPressu na fsociety.sk. **Force From Email**
zjednotí odosielateľa aj pre ostatné pluginy; zapni ho len ak je nová adresa vhodná
pre všetku poštu tohto WordPressu. Ak web už odosiela inú firemnú poštu, pred
zmenou treba pripraviť samostatné smerovanie licenčných správ. Tlačidlá predaja
na samostatnom HTML webe zostávajú smerované na sales@darkphish.sk.
Postup rozhrania: [oficiálny návod WP Mail SMTP](https://wpmailsmtp.com/docs/how-to-set-up-the-other-smtp-mailer-in-wp-mail-smtp/).

## 3. Over doručovanie

V administrácii pošty skontroluj SPF, DKIM a DMARC podľa pokynov Websupportu;
neprepisuj existujúce DNS záznamy naslepo. Cez **WP Mail SMTP → Tools → Email Test**
pošli test do vlastnej schránky a skontroluj doručenie aj spam. Úspech
`wp_mail()` sám osebe nepotvrdzuje doručenie. Pri chybe spojenia over povolené
odchádzajúce SMTP na hostingu fsociety.sk. Heslá a celé overovacie odkazy
nevkladaj do verejných logov, HTML stránky ani repozitára.

Po dokončení podmienok a nastavení formulára vykonaj celý test:
žiadosť → doručený overovací odkaz → výslovné potvrdenie → zobrazený kľúč →
aktivácia klienta. Aktuálny plugin posiela overovací odkaz; výsledný aktivačný
kľúč zobrazí na stránke po overení a osobitne ho e-mailom zatiaľ neposiela.

Do oznámenia o ochrane údajov doplň skutočné zmluvné subjekty pre hosting
(SlovakNET) a poštu (Websupport), miesta spracovania, zmluvné podmienky a lehoty
uchovávania. Značka produktu ani doména neurčuje právnu identitu prevádzkovateľa.
