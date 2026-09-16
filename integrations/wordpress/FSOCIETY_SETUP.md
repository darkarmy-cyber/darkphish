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

Nahraj ZIP verzie **0.1.1** cez **Pluginy → Pridať nový → Nahrať plugin**
a zvoľ nahradenie nainštalovanej verzie. Licencie a databázové tabuľky zostanú.

Cez HTTPS otvor **Nastavenia → Darkphish licensing**. Klikni na
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

- názov: napríklad `fsociety Darkphish`;
- hostname: `fsociety.sk` (pridaj `www.fsociety.sk`, iba ak ho používaš);
- režim: **Managed**.

Doménu nemusíš presúvať do Cloudflare a DNS nemeníš. Do `wp-config.php`, na
rovnaké miesto ako predchádzajúcu konštantu, vlož skutočné hodnoty:

```php
define('DARKPHISH_TURNSTILE_SITE_KEY', 'SEM_VLOZ_SITE_KEY');
define('DARKPHISH_TURNSTILE_SECRET', 'SEM_VLOZ_SECRET_KEY');
```

Zástupné hodnoty nahraď; Site key je verejný, Secret key zostáva na hostingu.
[Oficiálny postup](https://developers.cloudflare.com/turnstile/get-started/).

## 5. Registrácia a overenie

Vytvor stránku so shortcode `[darkphish_license]`. V nastaveniach pluginu
vyplň jej HTTPS URL, URL schválených licenčných podmienok a verziu podmienok.
API po dokončení konfigurácie používa základnú adresu
`https://fsociety.sk/wp-json/darkphish-license/v1`.

Ďalší krok je skúška doručenia overovacieho e-mailu a reálnej aktivácie Go
klienta. Samotné nahratie pluginu alebo vytvorenie kľúča tento test nenahrádza.
