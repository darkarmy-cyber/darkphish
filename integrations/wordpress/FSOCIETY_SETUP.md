# fsociety.sk — nastavenie bez SSH

Používateľ potvrdil WordPress 7.1, PHP 8.4.25 a túto štruktúru FTP:

```text
data/
log/
public_html/wp-config.php
stats/
tmp/
```

## 1. Súkromný priečinok

Vo FTP otvor `data` a vytvor `darkphish-private`. Na tomto novom priečinku
nastav práva **700**. Neupravuj práva ostatných hostingových adresárov.
Priečinok musí byť zapisovateľný používateľom PHP a nesmie byť sprístupnený
cez inú doménu, alias alebo symlink webového servera.

Podpisový súbor zatiaľ nevytváraj; vygeneruje ho aktualizovaný plugin.

## 2. Konfigurácia WordPressu

Uprav existujúci `public_html/wp-config.php`. Nasledujúci riadok vlož **pred**
`require_once ABSPATH . 'wp-settings.php';` (typicky nad poznámku „That's all,
stop editing!“). Ak konštanta už existuje, uprav ju, nevkladaj ju druhýkrát.
Nepridávaj ďalšie `<?php`.

```php
define('DARKPHISH_LICENSE_KEY_FILE', dirname(__DIR__) . '/data/darkphish-private/signing.json');
```

`dirname(__DIR__)` tu znamená rodiča `public_html`, preto netreba hádať
absolútnu serverovú cestu. Plugin aj tak skontroluje skutočné umiestnenie.

## 3. Aktualizácia pluginu a vytvorenie kľúča

Nahraj ZIP verzie **0.2.0** cez **Pluginy → Pridať nový → Nahrať plugin**
a zvoľ nahradenie nainštalovanej verzie. Licencie a databázové tabuľky zostanú.

Cez HTTPS otvor **DarkPhish → Settings**. Klikni na
**Vytvoriť podpisový kľúč na serveri**. Táto operácia nevypíše súkromný kľúč,
nevytvorí ho vo verejnom adresári a nikdy neprepíše existujúci súbor.

Po obnovení stránky sa zobrazí **Public verification keyring**. Pošli do chatu
iba tento verejný JSON; súbor `signing.json` neposielaj ani nesťahuj do chatu.

Ak vytvorenie zlyhá, pošli zobrazenú hlášku a verejné diagnostické cesty
(WordPress, verejný koreň, nakonfigurovaná cesta). Hosting môže potrebovať
povoliť prístup PHP do súkromného priečinka cez `open_basedir` alebo opraviť
vlastníka. Nenastavuj priečinok na 777 ako náhradu tejto opravy.

## 4. Turnstile bez zmeny DNS

V Cloudflare účte otvor **Turnstile → Add widget**:

- názov: napríklad `fsociety DarkPhish`;
- hostname: `darkphish.sk` pre HTML registráciu; existujúci `fsociety.sk` môže zostať;
- režim: **Managed**.

Doménu nemusíš presúvať do Cloudflare a DNS nemeníš. Do `wp-config.php`, na
rovnaké miesto ako predchádzajúcu konštantu, vlož skutočné hodnoty:

```php
define('DARKPHISH_TURNSTILE_SITE_KEY', 'SEM_VLOZ_SITE_KEY');
define('DARKPHISH_TURNSTILE_SECRET', 'SEM_VLOZ_SECRET_KEY');
```

Zástupné hodnoty nahraď; Site key je verejný, Secret key zostáva na hostingu.
[Oficiálny postup](https://developers.cloudflare.com/turnstile/get-started/).

## 5. HTML registrácia na darkphish.sk

WordPress aj podpisový kľúč zostávajú na fsociety.sk. Nahraj aktualizovaný
plugin 0.1.2 a do jeho wp-config.php pred wp-settings.php pridaj:

```php
define('DARKPHISH_LICENSE_REGISTRATION_ORIGIN', 'https://www.darkphish.sk');
```

Existujúce podpisové a Turnstile konštanty zachovaj. Nový podpisový kľúč
nevytváraj. Obsah balíka statickej stránky (`license/`) nahraj do verejného
koreňa **darkphish.sk**, takže vznikne `https://www.darkphish.sk/license/`.
Neprepisuj existujúci index hlavnej stránky. Na stránku nedávaj ďalšie skripty.

V nastaveniach pluginu ulož:
- Registration page URL: `https://www.darkphish.sk/license/`
- Community terms URL: skutočnú HTTPS stránku schválených licenčných podmienok
- Terms version: označenie verzie týchto podmienok

Do existujúceho Turnstile widgetu pridaj hostname `www.darkphish.sk`.
HTML formulár si verejné nastavenia načíta sám; žiadny secret doň nekopíruj.
Ak web presmerúva na www, najprv zjednoť kanonickú doménu a origin v konfigurácii.
Súbor otvor cez jeho HTTPS adresu, nie lokálne cez file://.

Ďalší krok je skúška doručenia overovacieho e-mailu a reálnej aktivácie Go
klienta. Overovací odkaz smeruje späť na darkphish.sk; kľúč sa zobrazí až
po výslovnom kliknutí, samotné otvorenie odkazu ho nevydá. Plugin používa API
`https://fsociety.sk/wp-json/darkphish-license/v1`.

Konštanta originu musí byť presne https://www.darkphish.sk, bez koncového lomítka alebo /license/. Registračná URL v nastaveniach pluginu je naopak celá adresa https://www.darkphish.sk/license/. Statická stránka a jej stavové/chybové hlášky sú v angličtine. Pri aktualizácii prepíš všetky tri súbory v license/; HTML odkazuje na novú verziu JS/CSS, aby prehliadač nepoužil staré texty z cache.
