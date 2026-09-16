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
   povolená doména `www.darkphish.sk` pre statický formulár (alebo `fsociety.sk` pre shortcode) a samostatne testovacia doména. DNS webu nemusí
   byť vedené cez Cloudflare. Site key je verejný, Secret key patrí iba na server.
6. Schválenú stránku podmienok Community licencie a označenie jej verzie.
   Plugin právne podmienky nevymýšľa; verejnú registráciu bez ich konfigurácie nepovolí.

## Vytvorenie podpisového kľúča na hostingu

Od verzie 0.1.1 **SSH nie je potrebné**. Najprv cez FTP alebo správcu hostingu
vytvor súkromný priečinok mimo všetkých verejných webových adresárov a zisti
jeho absolútnu serverovú cestu. Relatívna cesta z FTP nemusí zodpovedať serverovej.
Ak má FTP prístup iba k verejnému webu, súkromný priečinok musí pripraviť hosting.

1. Nahraj aktualizovaný ZIP a zvoľ nahradenie existujúcej verzie pluginu.
2. Nastav `DARKPHISH_LICENSE_KEY_FILE` vo `wp-config.php` na nový, ešte neexistujúci
   JSON súbor v tomto priečinku (ukážka nižšie).
3. Cez HTTPS otvor **Darkphish Licenses → Settings** a klikni na
   **Vytvoriť podpisový kľúč na serveri**. Funkcia vyžaduje oprávnenie správcu
   a platný formulárový nonce. Kľúč vznikne len na serveri s právami
   `0600`; do prehliadača sa jeho súkromná časť nikdy neposiela.
4. Po obnovení stránky skopíruj zobrazený **verejný keyring JSON**.

Tlačidlo neprepíše existujúci súbor ani nevytvorí kľúč vo verejnom adresári.
Neplatný/poškodený existujúci súbor treba najprv preveriť; automatická výmena
by mohla zneplatniť podpisy. Administrácia zobrazuje WordPress a document-root
cesty iba oprávnenému správcovi, aby vedel overiť umiestnenie súkromného súboru.

Nasledujúci CLI postup zostáva alternatívou pre hosting so SSH:

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

## HTML registrácia na darkphish.sk

Od verzie 0.1.2 môže verejná registrácia bežať na samostatnom HTML webe.
WordPress na fsociety.sk zostáva licenčným API; súkromné kľúče a databáza
zostávajú iba na tomto serveri. Do HTML webu nepridávaj PHP ani WordPress.

1. Vo wp-config.php nastav presný povolený origin bez koncového lomítka:

   ```php
   define('DARKPHISH_LICENSE_REGISTRATION_ORIGIN', 'https://www.darkphish.sk');
   ```

2. Nahraj obsah `static-site/license/` do priečinka `license` vo verejnom
   koreni HTML webu. Výsledná stránka je `https://www.darkphish.sk/license/`.
   Existujúcu hlavnú stránku neprepisuj. Na registráciu nepridávaj analytiku,
   reklamy ani iné skripty, ktoré môžu čítať token alebo zobrazený kľúč.
3. V administrácii WordPressu **Darkphish Licenses → Settings** nastav
   **Registration page URL** na `https://www.darkphish.sk/license/`, URL
   schválených podmienok a ich skutočnú verziu. Toto nastavenie určuje aj
   cieľ overovacieho e-mailu; ľubovoľné cudzie domény sú odmietnuté.
4. V existujúcom Turnstile widgete povoľ `darkphish.sk`. Secret key ponechaj
   iba vo wp-config.php. Formulár načíta Site key a aktuálne podmienky cez
   verejný `/public-config`; súkromný kľúč ani secret v tejto odpovedi nie sú.
5. Na hostingu/CDN vypni cache a zaznamenávanie tiel požiadaviek/odpovedí pre
   `/wp-json/darkphish-license/v1/*`. Žiadne tokeny, aktivačné kľúče ani celé
   overovacie e-maily nesmú skončiť v analytike alebo debug logoch.
6. Registrácia používa HTTPS, presný povolený origin, CORS preflight a žiadne
   WordPress cookies. Turnstile odpoveď musí mať očakávaný hostname a action.
   Iné WordPress API trasy si zachovávajú svoju pôvodnú CORS politiku.

Voliteľná registrácia na rovnakom WordPresse cez `[darkphish_license]`
zostáva podporovaná; vtedy použi jeho stránku a hostname Turnstile.
Origin bez hlavičky je povolený pre natívneho klienta; CORS nenahrádza
Turnstile, overenie e-mailu ani platné aktivačné/obnovovacie poverenia.

Produkčná základná URL API je `https://fsociety.sk/wp-json/darkphish-license/v1`.
Samotná konfigurácia nie je potvrdením živého testu pošty a aktivácie.

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

- `GET /public-config`: iba verejný Site key, URL/verzia podmienok a registračná URL.
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

## Rozpracované podmienky a pošta

Anglické návrhy podmienok a oznámenia o ochrane údajov sú v
[legal-drafts/](legal-drafts/README.md). Obsahujú výslovne označenú pripravovanú
spoločnosť a IČO `XXXX`; nie sú určené na publikovanie ani zapnutie registrácie.
Postup pre plánovanú licenčnú schránku `license@darkphish.sk` u Websupportu je v
[návode na SMTP](legal-drafts/MAIL_SETUP.sk.md).

## Darkphish Licenses 0.2.0

Samostatné menu sa zaradí priamo pod Zeus, ak je prítomný. Záložky Dashboard, Community, Professional, Enterprise a Settings zobrazujú skutočné štatistiky a stránkované licencie. Správca môže ručne vydať Professional/Enterprise licenciu, upraviť jej limity a platnosť alebo použiť revoke/restore/reset/renew. Predvolené limity sa upravujú v Settings a nevynucujú sa spätne na už vydaných licenciách. Ručné vydanie neoveruje platbu ani neposiela kľúč poštou. Kľúč sa zobrazí iba v odpovedi na vydanie.

Aktualizácia zachová názov priečinka darkphish-license, konštanty, nastavenia, podpisový súbor a existujúce licencie. Databáza sa rozšíri pri nasledujúcom načítaní pluginov; existujúce licencie sú Community. Verejné vydanie/obnova Community nemôžu prepísať komerčnú licenciu pre rovnaký e-mail. Zmeny limitov a zrušenie sa prejavia pri ďalšom podpísanom lease, nie v už vydanom offline zázname. Platená licencia potrebuje klienta s podporou danej edície; sama nepridáva ďalšie produktové funkcie.

Publikované predvoľby podľa dodaného index.html: Professional 250 používateľov a neobmedzené kampane; Enterprise oba limity neobmedzené. Platnosť sa pri ručnom vydaní zadáva podľa dohody (stránka rozlišuje mesačný a ročný spôsob platby). Hodnota Unlimited je v podpísanom protokole -1, povolená iba pri platených edíciách; vyžaduje aktualizovaný klient z PR #70. Registračná stránka nemá footer a používa vlastný responzívny štýl. Vycentrovaný nadpis obsahuje živý text DarkPhish s farebným prechodom; obrázkové logo sa nepoužíva. Farebný prechod je prevzatý z dodaného styles.css hlavnej stránky: 90deg, #1fd1dc → #60efda, rovnako ako „Built for defenders.“.

Aktualizácia registračnej stránky en6: pri šírke do 900 px je rozloženie jednosĺpcové, širšie obrazovky používajú dva pružné stĺpce. Vzhľad bol skontrolovaný pri 240–1920 px bez vodorovného pretekania. Stránka používa kompaktný Turnstile, aby sa zmestil aj do úzkeho formulára. Pri HTTP 503 z public-config zobrazí informáciu, že registrácia ešte nie je otvorená. Používateľ potvrdil, že podmienky a ich verzia v Settings nie sú nastavené; API preto registráciu správne nepovolí. SMTP sa pri načítaní tejto verejnej konfigurácie netestuje.

## Potvrdenie žiadosti a HTML e-mail — 0.2.1 / en7

Statická stránka po prijatí žiadosti zobrazí samostatnú obrazovku „Check your inbox“
s adresou, ďalším postupom a odkazom na zadanie inej adresy. Počas odosielania
blokuje duplicitné žiadosti, pri chybe zvýrazní a zameria hlásenie. Úspešné prijatie
žiadosti nie je tvrdením o doručení: odpoveď ostáva rovnaká aj pri limite pre
konkrétny e-mail, aby API neprezrádzalo stav adresy.

Plugin 0.2.1 posiela HTML e-mail s vloženými štýlmi, overovacím tlačidlom,
30-minútovou platnosťou a celým odkazom na skopírovanie. Textová alternatíva
sa pridáva cez dočasný, na konkrétnu správu obmedzený phpmailer_init hook.
Nie sú použité vzdialené obrázky, fonty ani sledovacie pixely. SMTP nastavenia
a odosielateľ ostávajú vo WP Mail SMTP; nemeníme globálny typ ostatných správ.
Podklad: [wp_mail](https://developer.wordpress.org/reference/functions/wp_mail/)
a [phpmailer_init](https://developer.wordpress.org/reference/hooks/phpmailer_init/).

Nasadenie potrebuje nový plugin ZIP aj tri súbory statickej stránky v license/.
Kľúč, databáza a nastavenia sa zachovajú. Používateľ už potvrdil prijatie pôvodného
textového overovacieho e-mailu; nový HTML formát ešte potrebuje kontrolu v jeho
poštovej aplikácii. Testy neposielajú skutočné e-maily.

## Oddelená registrácia a obnova — 0.2.2 / en8

Nová registrácia už nikdy nemení kľúč existujúcej Community licencie. Po overení
mailboxu vráti already_registered a ponúkne samostatnú obnovu. Jedna Community
licencia patrí jednej adrese bez rozlíšenia veľkosti písmen a okolitých medzier.
Platené edície ostávajú samostatné; aliasy rôznych adries sa svojvoľne nezlučujú.

Obnova začína na /license/?mode=recover. POST recover iba vytvorí žiadosť a odošle
potvrdenie; existenciu licencie skontroluje až POST verify po preukázaní vlastníctva
mailboxu. Odpoveď pri odoslaní je rovnaká pre existujúcu aj neexistujúcu adresu.
Oba režimy zdieľajú e-mailový limit. Účel žiadosti je uložený na serveri; parameter
mode v URL ovplyvňuje iba zobrazenie a nemôže zmeniť účel už vydaného tokenu.

Obnova platnej licencie mení iba hash aktivačného kľúča. Zachová ID, vytvorenie,
platnosť, limity, väzbu, obnovovací token aj prijaté podmienky. Náhradný kľúč sa
zobrazí a odošle do overenej schránky. Ak odoslanie kópie zlyhá, platný kľúč zostane
zobrazený na stránke; dátum platnosti sa nemení. Kľúč sa neukladá v čitateľnej podobe
do licenčnej databázy. SMTP logovanie tiel správ musí zostať vypnuté.
Chýbajúca licencia sa nevytvára, expirovaná sa nepredlžuje a odvolaná neobnovuje.

Schéma 3 pridáva účel k requests a overuje jedinečný index email_hash. Staršie
nevybavené odkazy sa považujú za registráciu, preto nemôžu implicitne nahradiť kľúč.
Vyhľadávanie kontroluje aj uloženú adresu pre historické nekanonické hashe.
Viac Community záznamov pre rovnakú adresu vyvolá upozornenie v Dashboard/Community
a výsledok support_required po overení. Nič sa automaticky nemaže ani nezlučuje.

Nasadenie: nahraď plugin rovnakého názvu priečinka balíkom 0.2.2 a nahraj tri nové
súbory license/. Inštalácia doplní schému pri ďalšom načítaní; súkromný kľúč a
nastavenia sa zachovajú. Skutočné údajné duplicitné riadky z produkcie neboli
sprístupnené; príčina ich zobrazenia sa bez tejto kontroly nepovažuje za potvrdenú.
