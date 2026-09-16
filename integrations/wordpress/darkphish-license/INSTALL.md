# WordPress licenčný server pre fsociety.sk

Plugin `darkphish-license` poskytuje serverovú časť protokolu klienta z PR #70.
Balík je určený najprv na testovacie nasadenie. Nahratie pluginu samo osebe
nevytvorí produkčný podpisový kľúč ani nezačne posielať správy používateľom.

## Čo treba pripraviť

1. Jednostránkovú inštaláciu WordPressu 6.8 alebo novšiu, PHP 8.2+ s rozšírením
   `sodium`, MySQL/MariaDB s InnoDB a HTTPS. Multisite zatiaľ nie je podporovaný.
2. Možnosť nahrať ZIP cez **Pluginy → Pridať nový → Nahrať plugin**.
3. Prístup správcu hostingu k súboru mimo verejného webového adresára a možnosť
   doplniť konštanty do `wp-config.php`. Heslá ani privátne kľúče neposielaj do chatu.
4. Fungujúce odosielanie e-mailov vo WordPresse. Plugin používa `wp_mail()`;
   prijatie správy poštovým serverom treba následne overiť v reálnej schránke.
5. Cloudflare Turnstile widget: **Turnstile → Add widget**, režim Managed,
   povolená doména `fsociety.sk` a samostatne testovacia doména. DNS webu nemusí
   byť vedené cez Cloudflare. Site key je verejný, Secret key patrí iba na server.
6. Schválenú stránku podmienok Community licencie a označenie jej verzie.
   Plugin právne podmienky nevymýšľa; verejnú registráciu bez ich konfigurácie nepovolí.

## Vytvorenie podpisového kľúča na hostingu

Príklad cesty `/home/ACCOUNT/private/darkphish-signing.json` musí správca
nahradiť skutočnou cestou **mimo document root** (napríklad mimo `public_html`).
Priečinok má byť súkromný a súbor čitateľný len používateľom PHP. Na Linuxe
plugin odmietne kľúč s právami pre skupinu alebo ostatných; nastav `0600`.

Na serveri, cez SSH alebo pomocou správcu hostingu:

```sh
umask 077
mkdir -p /home/ACCOUNT/private
chmod 700 /home/ACCOUNT/private
php /PATH/TO/wordpress/wp-content/plugins/darkphish-license/tools/create-key.php \
  /home/ACCOUNT/private/darkphish-signing.json DP-COM-2026-01
```

Nástroj odmieta prepísať existujúci súbor. Vypíše iba **verejný keyring JSON**.
Tento verejný JSON a key ID môžeš poslať do chatu. Súkromný JSON súbor neopúšťa
server a jeho zabezpečené zálohy. Staging musí mať vlastný kľúč a databázu.

Do `wp-config.php`, pred načítanie WordPressu, doplň:

```php
define('DARKPHISH_LICENSE_KEY_FILE', '/home/ACCOUNT/private/darkphish-signing.json');
define('DARKPHISH_TURNSTILE_SITE_KEY', 'PUBLIC_SITE_KEY');
define('DARKPHISH_TURNSTILE_SECRET', 'SERVER_ONLY_SECRET_KEY');
```

Tieto zástupné hodnoty nefungujú a nepatria do produkcie. Pri reverznom proxy
nastav dôveryhodné HTTPS a skutočnú klientsku IP na úrovni servera. Plugin
nepreberá ľubovoľné `X-Forwarded-*` hlavičky. `home` aj `siteurl` majú používať HTTPS.

## Dokončenie vo WordPresse

1. Nahraj a aktivuj ZIP. Plugin vytvorí vlastné tabuľky InnoDB s prefixom WordPressu.
2. Vytvor stránku napríklad `https://fsociety.sk/darkphish-licencia/` s blokom
   Shortcode obsahujúcim `[darkphish_license]`. Použi stránku bez reklamných
   a analytických skriptov, ktoré by mohli čítať obsah formulára.
3. V **Nastavenia → Darkphish licensing** ulož URL tejto stránky, URL podmienok
   a verziu podmienok. Skontroluj zobrazený verejný keyring a adresu API.
4. Na hostingu/CDN vypni cache a zaznamenávanie tiel požiadaviek/odpovedí pre
   `/wp-json/darkphish-license/v1/*`. Tokeny ani aktivačné kľúče nesmú skončiť
   v analytike, debug mail pluginoch alebo logoch.
5. Pri CSP povoľ Turnstile podľa oficiálneho návodu. Verejný formulár je
   zámerne na fsociety.sk; web darkphish.sk naň môže odkazovať. Iné originy nie
   sú potrebné pre Go klienta a automaticky sa nepovoľujú.

Plánovaná produkčná základná URL API je:

`https://fsociety.sk/wp-json/darkphish-license/v1`

Táto adresa bude funkčná až po inštalácii, konfigurácii a overení hostingu.

## Prepojenie klienta Darkphish

Verejný keyring z administrácie ulož na serveri aplikácie do
`/etc/darkphish/license-public-keys.json`. Obsahuje schému
`darkphish-license-keyring/v1` a mapu `keys` s 32-bajtovými verejnými kľúčmi
zakódovanými štandardným Base64. Do klienta nikdy nekopíruj podpisový súbor.

```json
{
  "license": {
    "service_url": "https://fsociety.sk/wp-json/darkphish-license/v1",
    "keyring_file": "/etc/darkphish/license-public-keys.json",
    "state_path": "/var/lib/darkphish/license-state.json",
    "refresh_interval_seconds": 3600
  }
}
```

Reštartuj klienta s implementáciou PR #70. Používateľ po overení e-mailu
jednorazovo uvidí aktivačný kľúč a vloží ho do **Settings → Licensing**.
Pri strate kľúča môže zopakovať overenie e-mailu. To nahradí pôvodný aktivačný
kľúč, zachová existujúcu aktiváciu a neobíde zrušenie ani expiráciu licencie.

## Protokol a životný cyklus

- `POST /request`: `email`, `terms_version`, `challenge_token`; po overení
  Turnstile pošle 30-minútový odkaz. Rovnaká odpoveď pre nové aj existujúce účty.
- `POST /verify`: `token`; jednorazovo vráti `license_key`, `license_id`, `expires_at`.
- `POST /activate`: `license_key`, `installation_id`, `product_version`;
  vráti `lease` (podpísaný envelope) a `refresh_token`.
- `POST /refresh`: `refresh_token`, `installation_id`, `product_version`;
  vráti `lease`. Obnovovací token zostáva stabilný; nová aktivácia ho vymení,
  reset alebo zrušenie ho zneplatní. Klient podporuje chýbajúci nový token pri obnove.
- Formát podpisu: Ed25519 nad presnými UTF-8 bajtmi payloadu; payload a podpis
  sa prenášajú ako Base64URL bez paddingu. Privátny kľúč nie je v databáze.
- Predvolená licencia: 365 dní, 100 používateľov, 1 aktívna kampaň, 1 inštalácia.
  Lease platí najviac 30 dní, grace ďalších najviac 30 dní; nikdy nepresiahnu
  koniec platnosti licencie. Vydané limity sú uložené pri konkrétnej licencii.
- Správca môže licenciu zrušiť, obnoviť jej stav, resetovať aktiváciu alebo
  predĺžiť platnosť o rok. Každá zmena zapisuje audit v tej istej transakcii.
- **Zrušenie zastaví vydávanie ďalších lease. Už vydaný offline lease ostáva
  platný do podpísaného grace termínu, najviac 60 dní od vydania.** Protokol v1
  nemá podpísaný revokačný objekt; odpoveď HTTP 403 sama osebe nevymaže dôveryhodný
  lease klienta. Rovnako reset neodoberie offline oprávnenie pôvodnej inštalácii.
- Rotácia podpisu: najprv distribuuj klientom verejné kľúče oboch key ID,
  potom prepnúť server na nový súkromný súbor. Starý verejný kľúč ponechaj do
  konca grace posledného starého lease. Kompromitácia vyžaduje aj aktualizáciu
  klientskych keyringov; centrálne HTTP odmietnutie nestačí pre offline klientov.

Overovacie a obnovovacie tokeny aj licenčné kľúče majú 256 bitov náhodnosti;
databáza uchováva iba ich SHA-256 odtlačky. Overovací token je vo fragmente URL,
nie v serverovom query logu. E-mail sa uchováva pri licencii. Audit obsahuje
ID licencie, akciu, ID správcu a čas, nikdy kľúč alebo token. Limity IP používajú
časovo ohraničený HMAC, nie surovú IP. Hodinový cron čistí expirované dočasné
záznamy; nastav systémové spúšťanie WP-Cron pri málo navštevovanom webe.
Deaktivácia nemaže licencie ani audit. Ich správu a prípadné vymazanie treba
zahrnúť do prevádzkového procesu hostingu; plugin nevytvára automatický export.

## Overenie pred produkciou

- Skutočný potvrdzovací e-mail dorazí a odkaz funguje iba raz, bez kliknutia
  sa licencia nevydá. Scanner e-mailov samotným otvorením GET licenciu nevydá.
- Go klient prijme podpis a po reštarte načíta aktiváciu; obnoví lease.
- Druhá odlišná inštalácia dostane odmietnutie; reset povolí novú aktiváciu.
- Zrušená a expirovaná licencia nedostane nový lease. Vyskúšaj odstávku API
  a následnú obnovu pri zachovaní posledného podpísaného lease.
- Neprihlásený používateľ ani bežný WordPress účet nemôže vykonať admin akcie.
- Verejné API nesmie vrátiť SQL, cestu súkromného kľúča ani raw serverové chyby.

CI používa skutočný WordPress 7.1, MySQL/InnoDB a izolované testovacie podpisy.
Odosielanie mailov a Turnstile sú v CI zachytené testovacími adaptérmi; doručenie
reálnej pošty, WAF, TLS a hostingové oprávnenia treba overiť na stagingu.

Referencie: [WordPress REST API](https://developer.wordpress.org/rest-api/extending-the-rest-api/adding-custom-endpoints/),
[wp_mail](https://developer.wordpress.org/reference/functions/wp_mail/),
[Turnstile server validation](https://developers.cloudflare.com/turnstile/get-started/server-side-validation/),
[PHP Ed25519 signing](https://www.php.net/manual/en/function.sodium-crypto-sign-detached.php).
