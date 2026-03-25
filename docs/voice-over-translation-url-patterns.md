# Паттерны URL из `vot.user.js` → Go-совместимые regexp

Ниже — соответствие `@match`/`@exclude` из userscript `voice-over-translation` и regexp в синтаксисе Go (`regexp` / RE2), которые можно использовать, например, в `source_rules[].pattern` или `source_rules[].patterns`.
При использовании в JSON-конфиге не забудьте экранировать обратные слэши (`\.` → `\\.`).

## @match

- `*://*.youtube.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*youtube\.com/.*`
- `*://*.youtube-nocookie.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*youtube-nocookie\.com/.*`
- `*://*.youtubekids.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*youtubekids\.com/.*`
- `*://*.twitch.tv/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*twitch\.tv/.*`
- `*://*.xvideos.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*xvideos\.com/.*`
- `*://*.xvideos-ar.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*xvideos-ar\.com/.*`
- `*://*.xvideos005.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*xvideos005\.com/.*`
- `*://*.xv-ru.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*xv-ru\.com/.*`
- `*://*.pornhub.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*pornhub\.com/.*`
- `*://*.pornhub.org/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*pornhub\.org/.*`
- `*://*.vk.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*vk\.com/.*`
- `*://*.vkvideo.ru/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*vkvideo\.ru/.*`
- `*://*.vk.ru/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*vk\.ru/.*`
- `*://*.vimeo.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*vimeo\.com/.*`
- `*://*.imdb.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*imdb\.com/.*`
- `*://*.9gag.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*9gag\.com/.*`
- `*://*.twitter.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*twitter\.com/.*`
- `*://*.x.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*x\.com/.*`
- `*://*.facebook.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*facebook\.com/.*`
- `*://*.rutube.ru/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*rutube\.ru/.*`
- `*://*.bilibili.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bilibili\.com/.*`
- `*://my.mail.ru/*` → `(?i)^https?://my\.mail\.ru/.*`
- `*://*.bitchute.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bitchute\.com/.*`
- `*://*.coursera.org/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*coursera\.org/.*`
- `*://*.udemy.com/course/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*udemy\.com/course/.*`
- `*://*.tiktok.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*tiktok\.com/.*`
- `*://*.douyin.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*douyin\.com/.*`
- `*://rumble.com/*` → `(?i)^https?://rumble\.com/.*`
- `*://*.eporner.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*eporner\.com/.*`
- `*://*.dailymotion.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*dailymotion\.com/.*`
- `*://*.ok.ru/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*ok\.ru/.*`
- `*://trovo.live/*` → `(?i)^https?://trovo\.live/.*`
- `*://disk.yandex.ru/*` → `(?i)^https?://disk\.yandex\.ru/.*`
- `*://disk.yandex.kz/*` → `(?i)^https?://disk\.yandex\.kz/.*`
- `*://disk.yandex.com/*` → `(?i)^https?://disk\.yandex\.com/.*`
- `*://disk.yandex.com.am/*` → `(?i)^https?://disk\.yandex\.com\.am/.*`
- `*://disk.yandex.com.ge/*` → `(?i)^https?://disk\.yandex\.com\.ge/.*`
- `*://disk.yandex.com.tr/*` → `(?i)^https?://disk\.yandex\.com\.tr/.*`
- `*://disk.yandex.by/*` → `(?i)^https?://disk\.yandex\.by/.*`
- `*://disk.yandex.az/*` → `(?i)^https?://disk\.yandex\.az/.*`
- `*://disk.yandex.co.il/*` → `(?i)^https?://disk\.yandex\.co\.il/.*`
- `*://disk.yandex.ee/*` → `(?i)^https?://disk\.yandex\.ee/.*`
- `*://disk.yandex.lt/*` → `(?i)^https?://disk\.yandex\.lt/.*`
- `*://disk.yandex.lv/*` → `(?i)^https?://disk\.yandex\.lv/.*`
- `*://disk.yandex.md/*` → `(?i)^https?://disk\.yandex\.md/.*`
- `*://disk.yandex.net/*` → `(?i)^https?://disk\.yandex\.net/.*`
- `*://disk.yandex.tj/*` → `(?i)^https?://disk\.yandex\.tj/.*`
- `*://disk.yandex.tm/*` → `(?i)^https?://disk\.yandex\.tm/.*`
- `*://disk.yandex.uz/*` → `(?i)^https?://disk\.yandex\.uz/.*`
- `*://youtube.googleapis.com/embed/*` → `(?i)^https?://youtube\.googleapis\.com/embed/.*`
- `*://*.banned.video/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*banned\.video/.*`
- `*://*.madmaxworld.tv/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*madmaxworld\.tv/.*`
- `*://*.weverse.io/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*weverse\.io/.*`
- `*://*.newgrounds.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*newgrounds\.com/.*`
- `*://*.egghead.io/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*egghead\.io/.*`
- `*://*.youku.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*youku\.com/.*`
- `*://*.archive.org/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*archive\.org/.*`
- `*://*.patreon.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*patreon\.com/.*`
- `*://*.reddit.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*reddit\.com/.*`
- `*://*.kodik.info/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*kodik\.info/.*`
- `*://*.kodik.biz/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*kodik\.biz/.*`
- `*://*.kodik.cc/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*kodik\.cc/.*`
- `*://*.kick.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*kick\.com/.*`
- `*://developer.apple.com/*` → `(?i)^https?://developer\.apple\.com/.*`
- `*://dev.epicgames.com/*` → `(?i)^https?://dev\.epicgames\.com/.*`
- `*://*.rapid-cloud.co/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*rapid-cloud\.co/.*`
- `*://odysee.com/*` → `(?i)^https?://odysee\.com/.*`
- `*://learning.sap.com/*` → `(?i)^https?://learning\.sap\.com/.*`
- `*://*.watchporn.to/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*watchporn\.to/.*`
- `*://*.linkedin.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*linkedin\.com/.*`
- `*://*.incestflix.net/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*incestflix\.net/.*`
- `*://*.incestflix.to/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*incestflix\.to/.*`
- `*://*.porntn.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*porntn\.com/.*`
- `*://*.dzen.ru/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*dzen\.ru/.*`
- `*://*.cloudflarestream.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*cloudflarestream\.com/.*`
- `*://*.loom.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*loom\.com/.*`
- `*://*.artstation.com/learning/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*artstation\.com/learning/.*`
- `*://*.rt.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*rt\.com/.*`
- `*://*.bitview.net/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bitview\.net/.*`
- `*://*.kickstarter.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*kickstarter\.com/.*`
- `*://*.thisvid.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*thisvid\.com/.*`
- `*://*.ign.com/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*ign\.com/.*`
- `*://*.bunkr.site/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.site/.*`
- `*://*.bunkr.black/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.black/.*`
- `*://*.bunkr.cat/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.cat/.*`
- `*://*.bunkr.media/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.media/.*`
- `*://*.bunkr.red/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.red/.*`
- `*://*.bunkr.ws/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.ws/.*`
- `*://*.bunkr.org/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.org/.*`
- `*://*.bunkr.sk/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.sk/.*`
- `*://*.bunkr.si/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.si/.*`
- `*://*.bunkr.su/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.su/.*`
- `*://*.bunkr.ci/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.ci/.*`
- `*://*.bunkr.cr/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.cr/.*`
- `*://*.bunkr.fi/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.fi/.*`
- `*://*.bunkr.ph/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.ph/.*`
- `*://*.bunkr.pk/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.pk/.*`
- `*://*.bunkr.ps/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.ps/.*`
- `*://*.bunkr.ru/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.ru/.*`
- `*://*.bunkr.la/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.la/.*`
- `*://*.bunkr.is/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.is/.*`
- `*://*.bunkr.to/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.to/.*`
- `*://*.bunkr.ac/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.ac/.*`
- `*://*.bunkr.ax/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bunkr\.ax/.*`
- `*://web.telegram.org/k/*` → `(?i)^https?://web\.telegram\.org/k/.*`
- `*://t2mc.toil.cc/*` → `(?i)^https?://t2mc\.toil\.cc/.*`
- `*://mylearn.oracle.com/*` → `(?i)^https?://mylearn\.oracle\.com/.*`
- `*://learn.deeplearning.ai/*` → `(?i)^https?://learn\.deeplearning\.ai/.*`
- `*://learn-staging.deeplearning.ai/*` → `(?i)^https?://learn-staging\.deeplearning\.ai/.*`
- `*://learn-dev.deeplearning.ai/*` → `(?i)^https?://learn-dev\.deeplearning\.ai/.*`
- `*://*.netacad.com/content/i2cs/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*netacad\.com/content/i2cs/.*`
- `*://*/*.mp4*` → `(?i)^https?://[^/]+/.*\.mp4.*`
- `*://*/*.webm*` → `(?i)^https?://[^/]+/.*\.webm.*`
- `*://*.yewtu.be/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*yewtu\.be/.*`
- `*://yt.artemislena.eu/*` → `(?i)^https?://yt\.artemislena\.eu/.*`
- `*://invidious.flokinet.to/*` → `(?i)^https?://invidious\.flokinet\.to/.*`
- `*://iv.melmac.space/*` → `(?i)^https?://iv\.melmac\.space/.*`
- `*://inv.nadeko.net/*` → `(?i)^https?://inv\.nadeko\.net/.*`
- `*://inv.tux.pizza/*` → `(?i)^https?://inv\.tux\.pizza/.*`
- `*://invidious.private.coffee/*` → `(?i)^https?://invidious\.private\.coffee/.*`
- `*://yt.drgnz.club/*` → `(?i)^https?://yt\.drgnz\.club/.*`
- `*://vid.puffyan.us/*` → `(?i)^https?://vid\.puffyan\.us/.*`
- `*://invidious.dhusch.de/*` → `(?i)^https?://invidious\.dhusch\.de/.*`
- `*://*.piped.video/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*piped\.video/.*`
- `*://piped.tokhmi.xyz/*` → `(?i)^https?://piped\.tokhmi\.xyz/.*`
- `*://piped.moomoo.me/*` → `(?i)^https?://piped\.moomoo\.me/.*`
- `*://piped.syncpundit.io/*` → `(?i)^https?://piped\.syncpundit\.io/.*`
- `*://piped.mha.fi/*` → `(?i)^https?://piped\.mha\.fi/.*`
- `*://watch.whatever.social/*` → `(?i)^https?://watch\.whatever\.social/.*`
- `*://piped.garudalinux.org/*` → `(?i)^https?://piped\.garudalinux\.org/.*`
- `*://efy.piped.pages.dev/*` → `(?i)^https?://efy\.piped\.pages\.dev/.*`
- `*://watch.leptons.xyz/*` → `(?i)^https?://watch\.leptons\.xyz/.*`
- `*://piped.lunar.icu/*` → `(?i)^https?://piped\.lunar\.icu/.*`
- `*://yt.dc09.ru/*` → `(?i)^https?://yt\.dc09\.ru/.*`
- `*://piped.mint.lgbt/*` → `(?i)^https?://piped\.mint\.lgbt/.*`
- `*://*.il.ax/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*il\.ax/.*`
- `*://piped.privacy.com.de/*` → `(?i)^https?://piped\.privacy\.com\.de/.*`
- `*://piped.esmailelbob.xyz/*` → `(?i)^https?://piped\.esmailelbob\.xyz/.*`
- `*://piped.projectsegfau.lt/*` → `(?i)^https?://piped\.projectsegfau\.lt/.*`
- `*://piped.in.projectsegfau.lt/*` → `(?i)^https?://piped\.in\.projectsegfau\.lt/.*`
- `*://piped.us.projectsegfau.lt/*` → `(?i)^https?://piped\.us\.projectsegfau\.lt/.*`
- `*://piped.privacydev.net/*` → `(?i)^https?://piped\.privacydev\.net/.*`
- `*://piped.palveluntarjoaja.eu/*` → `(?i)^https?://piped\.palveluntarjoaja\.eu/.*`
- `*://piped.smnz.de/*` → `(?i)^https?://piped\.smnz\.de/.*`
- `*://piped.adminforge.de/*` → `(?i)^https?://piped\.adminforge\.de/.*`
- `*://piped.qdi.fi/*` → `(?i)^https?://piped\.qdi\.fi/.*`
- `*://piped.hostux.net/*` → `(?i)^https?://piped\.hostux\.net/.*`
- `*://piped.chauvet.pro/*` → `(?i)^https?://piped\.chauvet\.pro/.*`
- `*://piped.jotoma.de/*` → `(?i)^https?://piped\.jotoma\.de/.*`
- `*://piped.pfcd.me/*` → `(?i)^https?://piped\.pfcd\.me/.*`
- `*://piped.frontendfriendly.xyz/*` → `(?i)^https?://piped\.frontendfriendly\.xyz/.*`
- `*://proxitok.pabloferreiro.es/*` → `(?i)^https?://proxitok\.pabloferreiro\.es/.*`
- `*://proxitok.pussthecat.org/*` → `(?i)^https?://proxitok\.pussthecat\.org/.*`
- `*://tok.habedieeh.re/*` → `(?i)^https?://tok\.habedieeh\.re/.*`
- `*://proxitok.esmailelbob.xyz/*` → `(?i)^https?://proxitok\.esmailelbob\.xyz/.*`
- `*://proxitok.privacydev.net/*` → `(?i)^https?://proxitok\.privacydev\.net/.*`
- `*://tok.artemislena.eu/*` → `(?i)^https?://tok\.artemislena\.eu/.*`
- `*://tok.adminforge.de/*` → `(?i)^https?://tok\.adminforge\.de/.*`
- `*://tt.vern.cc/*` → `(?i)^https?://tt\.vern\.cc/.*`
- `*://cringe.whatever.social/*` → `(?i)^https?://cringe\.whatever\.social/.*`
- `*://proxitok.lunar.icu/*` → `(?i)^https?://proxitok\.lunar\.icu/.*`
- `*://proxitok.privacy.com.de/*` → `(?i)^https?://proxitok\.privacy\.com\.de/.*`
- `*://peertube.1312.media/*` → `(?i)^https?://peertube\.1312\.media/.*`
- `*://tube.shanti.cafe/*` → `(?i)^https?://tube\.shanti\.cafe/.*`
- `*://*.bee-tube.fr/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*bee-tube\.fr/.*`
- `*://video.sadmin.io/*` → `(?i)^https?://video\.sadmin\.io/.*`
- `*://*.dalek.zone/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*dalek\.zone/.*`
- `*://review.peertube.biz/*` → `(?i)^https?://review\.peertube\.biz/.*`
- `*://*.peervideo.club/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*peervideo\.club/.*`
- `*://tube.la-dina.net/*` → `(?i)^https?://tube\.la-dina\.net/.*`
- `*://peertube.tmp.rcp.tf/*` → `(?i)^https?://peertube\.tmp\.rcp\.tf/.*`
- `*://*.peertube.su/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*peertube\.su/.*`
- `*://video.blender.org/*` → `(?i)^https?://video\.blender\.org/.*`
- `*://videos.viorsan.com/*` → `(?i)^https?://videos\.viorsan\.com/.*`
- `*://tube-sciences-technologies.apps.education.fr/*` → `(?i)^https?://tube-sciences-technologies\.apps\.education\.fr/.*`
- `*://tube-numerique-educatif.apps.education.fr/*` → `(?i)^https?://tube-numerique-educatif\.apps\.education\.fr/.*`
- `*://tube-arts-lettres-sciences-humaines.apps.education.fr/*` → `(?i)^https?://tube-arts-lettres-sciences-humaines\.apps\.education\.fr/.*`
- `*://*.beetoons.tv/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*beetoons\.tv/.*`
- `*://comics.peertube.biz/*` → `(?i)^https?://comics\.peertube\.biz/.*`
- `*://*.makertube.net/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*makertube\.net/.*`
- `*://*.poketube.fun/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*poketube\.fun/.*`
- `*://pt.sudovanilla.org/*` → `(?i)^https?://pt\.sudovanilla\.org/.*`
- `*://poke.ggtyler.dev/*` → `(?i)^https?://poke\.ggtyler\.dev/.*`
- `*://poke.uk2.littlekai.co.uk/*` → `(?i)^https?://poke\.uk2\.littlekai\.co\.uk/.*`
- `*://poke.blahai.gay/*` → `(?i)^https?://poke\.blahai\.gay/.*`
- `*://*.ricktube.ru/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*ricktube\.ru/.*`
- `*://*.coursehunter.net/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*coursehunter\.net/.*`
- `*://*.coursetrain.net/*` → `(?i)^https?://(?:[a-z0-9-]+\.)*coursetrain\.net/.*`

## @exclude

- `file://*/*.mp4*` → `(?i)^file://[^/]+/.*\.mp4.*`
- `file://*/*.webm*` → `(?i)^file://[^/]+/.*\.webm.*`
- `*://accounts.youtube.com/*` → `(?i)^https?://accounts\.youtube\.com/.*`

## Группировка по видеохостингам и фронтендам

Ниже — удобная группировка уже перечисленных выше паттернов по основным видеоплатформам и их альтернативным фронтендам/инстансам.
Эти группы можно использовать как подсказку при настройке `source_rules` (например, чтобы одним правилом покрывать сразу несколько инстансов одного сервиса).

### YouTube и совместимые фронтенды

**Основной домен YouTube**
- `*.youtube.com/*`
- `*.youtube-nocookie.com/*`
- `*.youtubekids.com/*`
- `youtube.googleapis.com/embed/*`

**Invidious / Yewtu / другие YouTube-фронтенды**
- `*.yewtu.be/*`
- `yt.artemislena.eu/*`
- `invidious.flokinet.to/*`
- `iv.melmac.space/*`
- `inv.nadeko.net/*`
- `inv.tux.pizza/*`
- `invidious.private.coffee/*`
- `yt.drgnz.club/*`
- `vid.puffyan.us/*`
- `invidious.dhusch.de/*`

**Piped и совместимые инстансы**
- `*.piped.video/*`
- `piped.tokhmi.xyz/*`
- `piped.moomoo.me/*`
- `piped.syncpundit.io/*`
- `piped.mha.fi/*`
- `watch.whatever.social/*`
- `piped.garudalinux.org/*`
- `efy.piped.pages.dev/*`
- `watch.leptons.xyz/*`
- `piped.lunar.icu/*`
- `yt.dc09.ru/*`
- `piped.mint.lgbt/*`
- `*.il.ax/*`
- `piped.privacy.com.de/*`
- `piped.esmailelbob.xyz/*`
- `piped.projectsegfau.lt/*`
- `piped.in.projectsegfau.lt/*`
- `piped.us.projectsegfau.lt/*`
- `piped.privacydev.net/*`
- `piped.palveluntarjoaja.eu/*`
- `piped.smnz.de/*`
- `piped.adminforge.de/*`
- `piped.qdi.fi/*`
- `piped.hostux.net/*`
- `piped.chauvet.pro/*`
- `piped.jotoma.de/*`
- `piped.pfcd.me/*`
- `piped.frontendfriendly.xyz/*`

**Poketube / Poke-инстансы**
- `*.poketube.fun/*`
- `pt.sudovanilla.org/*`
- `poke.ggtyler.dev/*`
- `poke.uk2.littlekai.co.uk/*`
- `poke.blahai.gay/*`
- `*.ricktube.ru/*`

### TikTok и фронтенды

**Официальные домены**
- `*.tiktok.com/*`
- `*.douyin.com/*`

**ProxiTok / Tok / альтернативные фронтенды TikTok**
- `proxitok.pabloferreiro.es/*`
- `proxitok.pussthecat.org/*`
- `tok.habedieeh.re/*`
- `proxitok.esmailelbob.xyz/*`
- `proxitok.privacydev.net/*`
- `tok.artemislena.eu/*`
- `tok.adminforge.de/*`
- `tt.vern.cc/*`
- `cringe.whatever.social/*`
- `proxitok.lunar.icu/*`
- `proxitok.privacy.com.de/*`

### Twitch и стриминговые платформы

- `*.twitch.tv/*`
- `trovo.live/*`
- `*.kick.com/*`

### Российские соцсети и видеохостинги

- `*.vk.com/*`
- `*.vkvideo.ru/*`
- `*.vk.ru/*`
- `*.ok.ru/*`
- `*.rutube.ru/*`
- `*.dzen.ru/*`
- `my.mail.ru/*`

### Международные видеохостинги

- `*.vimeo.com/*`
- `*.bilibili.com/*`
- `*.dailymotion.com/*`
- `rumble.com/*`
- `*.bitchute.com/*`
- `odysee.com/*`
- `*.newgrounds.com/*`
- `*.youku.com/*`
- `*.archive.org/*`

### Социальные сети и контент-платформы

- `*.twitter.com/*`, `*.x.com/*`
- `*.facebook.com/*`
- `*.9gag.com/*`
- `*.reddit.com/*`
- `*.linkedin.com/*`
- `*.patreon.com/*`
- `web.telegram.org/k/*`

### Образовательные платформы и документация

- `*.coursera.org/*`
- `*.udemy.com/course/*`
- `*.egghead.io/*`
- `learning.sap.com/*`
- `mylearn.oracle.com/*`
- `learn.deeplearning.ai/*`, `learn-staging.deeplearning.ai/*`, `learn-dev.deeplearning.ai/*`
- `*.netacad.com/content/i2cs/*`
- `developer.apple.com/*`
- `dev.epicgames.com/*`
- `*.artstation.com/learning/*`
- `learning.sap.com/*`

### PeerTube и совместимые инстансы

- `peertube.1312.media/*`
- `tube.shanti.cafe/*`
- `*.bee-tube.fr/*`
- `video.sadmin.io/*`
- `*.dalek.zone/*`
- `review.peertube.biz/*`
- `*.peervideo.club/*`
- `tube.la-dina.net/*`
- `peertube.tmp.rcp.tf/*`
- `*.peertube.su/*`
- `video.blender.org/*`
- `videos.viorsan.com/*`
- `tube-sciences-technologies.apps.education.fr/*`
- `tube-numerique-educatif.apps.education.fr/*`
- `tube-arts-lettres-sciences-humaines.apps.education.fr/*`
- `*.beetoons.tv/*`
- `comics.peertube.biz/*`
- `*.makertube.net/*`

### Яндекс.Диск и вспомогательные сервисы

- `disk.yandex.*` (все паттерны `disk.yandex.ru`, `.kz`, `.com`, `.com.am`, `.com.ge`, `.com.tr`, `.by`, `.az`, `.co.il`, `.ee`, `.lt`, `.lv`, `.md`, `.net`, `.tj`, `.tm`, `.uz`)
- `t2mc.toil.cc/*`

### Прочие видеосайты и хостинги

- `*.imdb.com/*`
- `*.kodik.info/*`, `*.kodik.biz/*`, `*.kodik.cc/*`
- `*.rt.com/*`
- `*.ign.com/*`
- `*.banned.video/*`, `*.madmaxworld.tv/*`
- `*.loom.com/*`
- `*.cloudflarestream.com/*`
- `*.bitview.net/*`
- `*.kickstarter.com/*`
- `*.thisvid.com/*`
- `*.bunkr.*` (все перечисленные домены bunkr)
- `*.beetoons.tv/*`

### Вспомогательные общие паттерны

- `*://*/*.mp4*`
- `*://*/*.webm*`

> Все перечисленные группы являются лишь логической надстройкой над базовым списком `@match`/`@exclude` выше и не влияют на фактические regexp.
> При добавлении новых источников в конфиг `source_rules` имеет смысл ориентироваться на эти группы, чтобы одинаково обрабатывать альтернативные инстансы одного и того же видеохостинга.
