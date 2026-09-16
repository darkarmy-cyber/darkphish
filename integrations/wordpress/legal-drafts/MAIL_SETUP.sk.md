# Nastavenie overovacích e-mailov

Stav k 16. 9. 2026: WordPress pošta ešte nie je nakonfigurovaná. Používateľ potvrdil, že schránka `info@darkarmy.sk` existuje u SlovakNETu. Tento návod
nemení nastavenia servera ani neposiela testovacie správy.

## 1. Použi potvrdenú schránku info@darkarmy.sk

Schránka je potvrdená. V administrácii poskytovateľa ešte over povolené SMTP.
Heslo zadáš iba do serverového nastavenia pošty, do chatu ho neposielaj.

## 2. Nastav transport WordPressu

Licenčný plugin používa štandardné `wp_mail()`. Najprv skontroluj, či WordPress
už má plugin na SMTP; nepoužívaj súčasne dva rôzne transporty. Ak ho nemá,
nainštaluj **WP Mail SMTP by WPForms** a v **WP Mail SMTP → Settings → General** zvoľ **Other SMTP**. Nastavenie bude ovplyvňovať poštu celého WordPressu na fsociety.sk. Nastav From Email na `info@darkarmy.sk` a From Name na `DarkPhish`. Force From Email zjednotí odosielateľa aj pre ostatné pluginy; použi ho iba vtedy, ak je táto adresa vhodná pre všetku poštu webu. Postup vychádza z [oficiálneho návodu pluginu](https://wpmailsmtp.com/docs/how-to-set-up-the-other-smtp-mailer-in-wp-mail-smtp/).

Pre schránky využívajúce poštovú službu SlovakNET/ZONER uvádza oficiálna
[nápoveda](https://napoveda.slovaknet.sk/clanek/nastaveni-imap-pop3-smtp/):

| Nastavenie | Hodnota |
| --- | --- |
| SMTP server | `smtp.zoner.com` |
| Port a šifrovanie | `587` + STARTTLS, alternatívne `465` + SSL/TLS |
| Overovanie | Zapnuté |
| Používateľ | `info@darkarmy.sk` |
| Heslo | Heslo schránky, zadané súkromne na serveri |
| Odosielateľ | `info@darkarmy.sk` |
| Zobrazované meno | DarkPhish |

Tieto hodnoty platia len vtedy, keď daná schránka používa túto službu.
Certifikáty sa musia overovať; nezapínaj nešifrované pripojenie. Heslo
nedávaj do HTML stránky, repozitára ani verejného keyringu. Overovacie odkazy
sú citlivé: nepovoľuj zbytočné ukladanie celých správ do mailových logov.

## 3. Over doručovanie

V administrácii pošty skontroluj SPF, DKIM a DMARC podľa pokynov poskytovateľa;
neprepisuj existujúce DNS záznamy naslepo. SlovakNET opisuje tieto mechanizmy
v [pokynoch k bezpečnému e-mailu](https://www.slovaknet.sk/e-maily/zasady-bezpecneho-emailu).
Potom cez **WP Mail SMTP → Tools → Email Test** odošli test do vlastnej schránky a skontroluj doručenie
aj spam. Úspech `wp_mail()` sám osebe nepotvrdzuje doručenie.

Po dokončení podmienok a nastavení formulára vykonaj jeden celý test:
žiadosť → doručený overovací odkaz → výslovné potvrdenie → zobrazený kľúč →
aktivácia klienta. Aktuálny plugin posiela overovací odkaz; výsledný aktivačný
kľúč zobrazí na stránke po overení a osobitne ho e-mailom zatiaľ neposiela.

Do oznámenia o ochrane údajov následne doplň skutočného poskytovateľa pošty,
miesta spracovania, príslušné zmluvné podmienky a lehoty uchovávania.
