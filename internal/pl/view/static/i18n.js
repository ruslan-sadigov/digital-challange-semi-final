// Multi-language support (az, en). Static markup uses data-i18n* attributes; JS uses t(key, vars).
const I18N = {
  en: {
    'app.name': 'Smart City Transportation',
    'lang.label': 'Language',
    'theme.dark': 'Dark theme', 'theme.light': 'Light theme',
    'nav.signin': 'Sign in', 'nav.register': 'Register', 'nav.signout': 'Sign out', 'nav.operator': 'Operator',
    'who.guest': 'Guest',
    'loc.share': 'Share my location', 'loc.stop': 'Stop sharing',
    'loc.test': 'Test location', 'loc.test.title': 'Click on the map to simulate where you are', 'loc.picking': 'Click on the map...',
    'loc.pick.toast': 'Click on the map to place your test location',
    'loc.denied': 'Location permission denied', 'loc.unavailable': 'Geolocation is not available in this browser',
    'test.title': 'Test location', 'test.clear': 'Clear',
    'test.hint': 'Drag the pin to move it. It replaces your GPS position for sharing, reports and ETA.',
    'test.nearest': 'Nearest stop', 'test.walk': 'Walk', 'test.arrives': 'Bus {bus} arrives', 'test.road': 'Road distance',
    'test.make': 'You can make it', 'test.leave': 'Leave within {t}',
    'test.miss': 'You will miss this bus', 'test.miss.why': 'The walk is longer than its ETA',
    'test.soonest': 'Soonest bus you can catch: {bus} in {t}',
    'test.closest': 'Closest stops. Select a bus to compare its ETA with your walk.',
    'test.none.title': 'No stop nearby', 'test.none.text': 'There is no bus stop within 800 m. Drag the pin closer to a road.',
    'test.noeta': 'No ETA available for this bus yet',
    'test.pin': 'Test location (drag to move)',
    'search.placeholder': 'Search bus or route, e.g. 65', 'search.label': 'Search buses',
    'buses.title': 'Buses', 'buses.count': '{n} buses', 'buses.count1': '1 bus', 'buses.all': '← All buses',
    'buses.empty.title': 'No buses found', 'buses.empty.text': 'Try a route number such as 65, or clear the search.',
    'route.label': 'Route {code}', 'route.full': 'Route {code}: {name}', 'route.none': 'No route assigned',
    'bus.passengers': 'Estimated passengers: {n} / {cap}',
    'bus.temporary': 'temporary route',
    'bus.joining': 'On its way to route {to} to relieve crowding, {km} km away. Still serving route {home} until it joins.',
    'bus.rerouted': 'Temporarily serving route {to} (normally {home}) to relieve crowding.',
    'bus.until': 'Until {t}.',
    'bus.tooltip': '{name} · route {code} · {density}',
    'density.ask': 'How crowded is it?', 'density.report': 'Report density',
    'density.thanks': 'Thanks, your report was recorded.', 'density.sent': 'Density report sent',
    'stops.title': 'Stops · select one for ETA',
    'eta.toast': 'ETA: {t} ({km} km)',
    'level.low': 'Low', 'level.medium': 'Medium', 'level.high': 'High', 'level.unknown': 'Unknown',
    'unit.s': 's', 'unit.min': 'min', 'unit.h': 'h',
    // operator
    'op.heatmap': 'Heatmap', 'op.notify': 'Enable notifications',
    'op.notify.on': 'Notifications enabled', 'op.notify.off': 'Notifications blocked', 'op.notify.unsupported': 'This browser does not support notifications',
    'tab.alerts': 'Alerts', 'tab.routes': 'Routes', 'tab.buses': 'Buses', 'tab.stations': 'Stations',
    'alerts.active': 'Active reroutes', 'alerts.anomalies': 'Anomalies', 'alerts.anomalies.hint': 'Route High for 10+ min',
    'alerts.active.empty.title': 'No active reroutes', 'alerts.active.empty.text': 'Approved reroutes stay here until you end them or their interval runs out.',
    'alerts.empty.title': 'All routes are flowing', 'alerts.empty.text': 'You will be alerted when a route stays High for the threshold.',
    'alerts.move': 'Move {bus} from {route}', 'alerts.nobus': 'No low-density bus available to move.',
    'alerts.since': 'High since {t} · detected {d}', 'alerts.resolved': ' · resolved {t}',
    'alerts.ends': 'Ends at {t}', 'alerts.manual': 'Runs until you end it',
    'alerts.interval': 'Reroute interval',
    'dur.0': 'Until I end it', 'dur.15': '15 min', 'dur.30': '30 min', 'dur.60': '1 hour', 'dur.120': '2 hours', 'dur.240': '4 hours',
    'btn.approve': 'Approve', 'btn.reject': 'Reject', 'btn.end': 'End reroute', 'btn.edit': 'Edit', 'btn.delete': 'Delete', 'btn.clear': 'Clear',
    'status.open': 'Open', 'status.resolved': 'Resolved', 'status.suggested': 'Suggested', 'status.active': 'Active',
    'status.rejected': 'Rejected', 'status.ended': 'Ended', 'status.expired': 'Expired',
    'reroute.approved': 'Reroute approved', 'reroute.rejected': 'Reroute rejected', 'reroute.ended': 'Reroute ended',
    'alert.opened.title': 'Route {code}: high density', 'alert.resolved.title': 'Route {code}: resolved',
    'alert.opened': 'Route {code} has been High for {d}. {n} reroute suggestion(s) ready.',
    'alert.resolved': 'Route {code} density is back below High. Active reroutes continue until they expire or you end them.',
    'routes.title': 'Routes', 'routes.live': 'Live density', 'routes.stops': '{n} stops', 'routes.highfor': 'High for {d}',
    'routes.new': 'New route', 'routes.edit': 'Edit route {code}', 'routes.code': 'Code, e.g. 65', 'routes.name': 'Name',
    'routes.help': 'Select stations on the map in travel order. The road path is planned automatically.',
    'routes.save': 'Save route', 'routes.undo': 'Remove last stop', 'routes.saved': 'Route saved', 'routes.confirm': 'Delete this route?',
    'busf.new': 'New bus', 'busf.edit': 'Edit bus {name}', 'busf.name': 'Name or plate', 'busf.capacity': 'Passenger capacity',
    'busf.noroute': 'No route', 'busf.save': 'Save bus', 'busf.saved': 'Bus saved', 'busf.confirm': 'Delete this bus?',
    'busf.temporary': '(temporary)', 'busf.empty.title': 'No buses yet', 'busf.empty.text': 'Add your first bus with the form below.',
    'st.new': 'New station', 'st.help': 'Click on the map to set the position.', 'st.name': 'Name', 'st.lat': 'Latitude', 'st.lng': 'Longitude',
    'st.add': 'Add station', 'st.added': 'Station added', 'st.title': 'Stations', 'st.count': '{n} stops', 'st.confirm': 'Delete this station?',
    'st.empty.title': 'No stations yet', 'st.empty.text': 'Click the map to place your first station.',
    // auth
    'auth.signin.title': 'Welcome back', 'auth.signin.lead': 'Sign in to report bus density and see your trips.',
    'auth.email': 'Email', 'auth.password': 'Password', 'auth.confirm': 'Confirm password', 'auth.pwhelp': 'At least 6 characters.',
    'auth.signin.btn': 'Sign in', 'auth.noaccount': 'No account?', 'auth.create': 'Create one', 'auth.guest': 'Continue as guest',
    'auth.register.title': 'Create your account', 'auth.register.lead': 'A passenger account lets your density reports count more.',
    'auth.register.btn': 'Create account', 'auth.already': 'Already registered?',
    'auth.op.title': 'Operator sign in', 'auth.op.lead': 'For transport operators. Accounts are created by an existing operator.',
    'auth.op.email': 'Operator email', 'auth.op.btn': 'Sign in to operator panel', 'auth.back': 'Back to the passenger map',
    'auth.mismatch': 'Passwords do not match',
  },
  az: {
    'app.name': 'Ağıllı Şəhər Nəqliyyatı',
    'lang.label': 'Dil',
    'theme.dark': 'Qaranlıq tema', 'theme.light': 'İşıqlı tema',
    'nav.signin': 'Daxil ol', 'nav.register': 'Qeydiyyat', 'nav.signout': 'Çıxış', 'nav.operator': 'Operator',
    'who.guest': 'Qonaq',
    'loc.share': 'Məkanımı paylaş', 'loc.stop': 'Paylaşmanı dayandır',
    'loc.test': 'Test məkanı', 'loc.test.title': 'Olduğunuz yeri simulyasiya etmək üçün xəritəyə klikləyin', 'loc.picking': 'Xəritəyə klikləyin...',
    'loc.pick.toast': 'Test məkanını yerləşdirmək üçün xəritəyə klikləyin',
    'loc.denied': 'Məkan icazəsi verilmədi', 'loc.unavailable': 'Bu brauzerdə məkan təyini mümkün deyil',
    'test.title': 'Test məkanı', 'test.clear': 'Təmizlə',
    'test.hint': 'Yerini dəyişmək üçün nişanı sürükləyin. O, paylaşma, sıxlıq məlumatı və gəliş vaxtı üçün GPS mövqeyinizi əvəz edir.',
    'test.nearest': 'Ən yaxın dayanacaq', 'test.walk': 'Piyada', 'test.arrives': '{bus} avtobusu gəlir', 'test.road': 'Yol məsafəsi',
    'test.make': 'Çatacaqsınız', 'test.leave': '{t} ərzində yola düşün',
    'test.miss': 'Bu avtobusa çatmayacaqsınız', 'test.miss.why': 'Piyada yol avtobusun gəlişindən uzun çəkir',
    'test.soonest': 'Çata biləcəyiniz ən tez avtobus: {bus}, {t} sonra',
    'test.closest': 'Ən yaxın dayanacaqlar. Gəliş vaxtını piyada yolunuzla müqayisə etmək üçün avtobus seçin.',
    'test.none.title': 'Yaxınlıqda dayanacaq yoxdur', 'test.none.text': '800 m radiusda avtobus dayanacağı yoxdur. Nişanı yola yaxınlaşdırın.',
    'test.noeta': 'Bu avtobus üçün hələ gəliş vaxtı yoxdur',
    'test.pin': 'Test məkanı (sürükləyin)',
    'search.placeholder': 'Avtobus və ya marşrut axtarın, məs. 65', 'search.label': 'Avtobus axtarışı',
    'buses.title': 'Avtobuslar', 'buses.count': '{n} avtobus', 'buses.count1': '1 avtobus', 'buses.all': '← Bütün avtobuslar',
    'buses.empty.title': 'Avtobus tapılmadı', 'buses.empty.text': '65 kimi marşrut nömrəsini yoxlayın və ya axtarışı təmizləyin.',
    'route.label': '{code} marşrutu', 'route.full': '{code} marşrutu: {name}', 'route.none': 'Marşrut təyin edilməyib',
    'bus.passengers': 'Təxmini sərnişin sayı: {n} / {cap}',
    'bus.temporary': 'müvəqqəti marşrut',
    'bus.joining': 'Sıxlığı azaltmaq üçün {to} marşrutuna gedir, {km} km qalıb. Qoşulana qədər {home} marşrutuna xidmət edir.',
    'bus.rerouted': 'Sıxlığı azaltmaq üçün müvəqqəti olaraq {to} marşrutuna xidmət edir (adətən {home}).',
    'bus.until': '{t}-dək.',
    'bus.tooltip': '{name} · {code} marşrutu · {density}',
    'density.ask': 'Nə qədər sıxdır?', 'density.report': 'Sıxlıq bildir',
    'density.thanks': 'Təşəkkürlər, məlumatınız qeydə alındı.', 'density.sent': 'Sıxlıq məlumatı göndərildi',
    'stops.title': 'Dayanacaqlar · gəliş vaxtı üçün seçin',
    'eta.toast': 'Gəliş vaxtı: {t} ({km} km)',
    'level.low': 'Aşağı', 'level.medium': 'Orta', 'level.high': 'Yüksək', 'level.unknown': 'Naməlum',
    'unit.s': 'san', 'unit.min': 'dəq', 'unit.h': 'saat',
    'op.heatmap': 'İstilik xəritəsi', 'op.notify': 'Bildirişləri aktiv et',
    'op.notify.on': 'Bildirişlər aktivdir', 'op.notify.off': 'Bildirişlər bloklanıb', 'op.notify.unsupported': 'Bu brauzer bildirişləri dəstəkləmir',
    'tab.alerts': 'Xəbərdarlıq', 'tab.routes': 'Marşrut', 'tab.buses': 'Avtobus', 'tab.stations': 'Dayanacaq',
    'alerts.active': 'Aktiv yönləndirmələr', 'alerts.anomalies': 'Anomaliyalar', 'alerts.anomalies.hint': 'Marşrut 10+ dəq yüksək sıxlıqda',
    'alerts.active.empty.title': 'Aktiv yönləndirmə yoxdur', 'alerts.active.empty.text': 'Təsdiqlənmiş yönləndirmələr siz bitirənə və ya müddəti bitənə qədər burada qalır.',
    'alerts.empty.title': 'Bütün marşrutlarda vəziyyət normaldır', 'alerts.empty.text': 'Marşrut həddən artıq müddət yüksək sıxlıqda qalanda xəbərdar ediləcəksiniz.',
    'alerts.move': '{bus} avtobusunu {route} marşrutundan köçür', 'alerts.nobus': 'Köçürmək üçün az sıxlıqlı avtobus yoxdur.',
    'alerts.since': '{t}-dən yüksək · aşkarlandı {d}', 'alerts.resolved': ' · həll edildi {t}',
    'alerts.ends': '{t}-də bitir', 'alerts.manual': 'Siz bitirənə qədər davam edir',
    'alerts.interval': 'Yönləndirmə müddəti',
    'dur.0': 'Mən bitirənə qədər', 'dur.15': '15 dəq', 'dur.30': '30 dəq', 'dur.60': '1 saat', 'dur.120': '2 saat', 'dur.240': '4 saat',
    'btn.approve': 'Təsdiqlə', 'btn.reject': 'Rədd et', 'btn.end': 'Yönləndirməni bitir', 'btn.edit': 'Redaktə', 'btn.delete': 'Sil', 'btn.clear': 'Təmizlə',
    'status.open': 'Açıq', 'status.resolved': 'Həll edildi', 'status.suggested': 'Təklif', 'status.active': 'Aktiv',
    'status.rejected': 'Rədd edildi', 'status.ended': 'Bitdi', 'status.expired': 'Vaxtı keçdi',
    'reroute.approved': 'Yönləndirmə təsdiqləndi', 'reroute.rejected': 'Yönləndirmə rədd edildi', 'reroute.ended': 'Yönləndirmə bitdi',
    'alert.opened.title': '{code} marşrutu: yüksək sıxlıq', 'alert.resolved.title': '{code} marşrutu: həll edildi',
    'alert.opened': '{code} marşrutu {d} yüksək sıxlıqdadır. {n} yönləndirmə təklifi hazırdır.',
    'alert.resolved': '{code} marşrutunda sıxlıq yenidən normallaşdı. Aktiv yönləndirmələr müddəti bitənə və ya siz bitirənə qədər davam edir.',
    'routes.title': 'Marşrutlar', 'routes.live': 'Canlı sıxlıq', 'routes.stops': '{n} dayanacaq', 'routes.highfor': '{d} yüksək sıxlıqda',
    'routes.new': 'Yeni marşrut', 'routes.edit': '{code} marşrutunu redaktə et', 'routes.code': 'Kod, məs. 65', 'routes.name': 'Ad',
    'routes.help': 'Xəritədə dayanacaqları hərəkət ardıcıllığı ilə seçin. Yol avtomatik planlaşdırılır.',
    'routes.save': 'Marşrutu saxla', 'routes.undo': 'Son dayanacağı sil', 'routes.saved': 'Marşrut saxlanıldı', 'routes.confirm': 'Bu marşrut silinsin?',
    'busf.new': 'Yeni avtobus', 'busf.edit': '{name} avtobusunu redaktə et', 'busf.name': 'Ad və ya dövlət nömrəsi', 'busf.capacity': 'Sərnişin tutumu',
    'busf.noroute': 'Marşrut yoxdur', 'busf.save': 'Avtobusu saxla', 'busf.saved': 'Avtobus saxlanıldı', 'busf.confirm': 'Bu avtobus silinsin?',
    'busf.temporary': '(müvəqqəti)', 'busf.empty.title': 'Hələ avtobus yoxdur', 'busf.empty.text': 'İlk avtobusu aşağıdakı forma ilə əlavə edin.',
    'st.new': 'Yeni dayanacaq', 'st.help': 'Mövqeyi təyin etmək üçün xəritəyə klikləyin.', 'st.name': 'Ad', 'st.lat': 'Enlik', 'st.lng': 'Uzunluq',
    'st.add': 'Dayanacaq əlavə et', 'st.added': 'Dayanacaq əlavə edildi', 'st.title': 'Dayanacaqlar', 'st.count': '{n} dayanacaq', 'st.confirm': 'Bu dayanacaq silinsin?',
    'st.empty.title': 'Hələ dayanacaq yoxdur', 'st.empty.text': 'İlk dayanacağı yerləşdirmək üçün xəritəyə klikləyin.',
    'auth.signin.title': 'Xoş gəlmisiniz', 'auth.signin.lead': 'Avtobus sıxlığını bildirmək və səfərlərinizi görmək üçün daxil olun.',
    'auth.email': 'E-poçt', 'auth.password': 'Şifrə', 'auth.confirm': 'Şifrəni təsdiqləyin', 'auth.pwhelp': 'Ən azı 6 simvol.',
    'auth.signin.btn': 'Daxil ol', 'auth.noaccount': 'Hesabınız yoxdur?', 'auth.create': 'Yaradın', 'auth.guest': 'Qonaq kimi davam edin',
    'auth.register.title': 'Hesab yaradın', 'auth.register.lead': 'Sərnişin hesabı ilə sıxlıq məlumatlarınız daha çox nəzərə alınır.',
    'auth.register.btn': 'Hesab yarat', 'auth.already': 'Artıq qeydiyyatdan keçmisiniz?',
    'auth.op.title': 'Operator girişi', 'auth.op.lead': 'Nəqliyyat operatorları üçün. Hesablar mövcud operator tərəfindən yaradılır.',
    'auth.op.email': 'Operator e-poçtu', 'auth.op.btn': 'Operator panelinə daxil ol', 'auth.back': 'Sərnişin xəritəsinə qayıt',
    'auth.mismatch': 'Şifrələr uyğun gəlmir',
  },
};

// Server error messages (English) mapped to translation strings.
const SERVER_ERRORS = {
  'invalid credentials': { en: 'Invalid email or password', az: 'E-poçt və ya şifrə yanlışdır' },
  'operators sign in on the operator tab': { en: 'Operators sign in on the Operator tab', az: 'Operatorlar "Operator" bölməsindən daxil olur' },
  'this is not an operator account': { en: 'This is not an operator account', az: 'Bu operator hesabı deyil' },
  'passwords do not match': { en: 'Passwords do not match', az: 'Şifrələr uyğun gəlmir' },
  'a valid email is required': { en: 'A valid email is required', az: 'Düzgün e-poçt ünvanı daxil edin' },
  'password must be at least 6 characters': { en: 'Password must be at least 6 characters', az: 'Şifrə ən azı 6 simvol olmalıdır' },
  'email already registered': { en: 'This email is already registered', az: 'Bu e-poçt artıq qeydiyyatdan keçib' },
  'you are too far from this bus to report its density': { en: 'You are too far from this bus to report its density', az: 'Bu avtobusun sıxlığını bildirmək üçün ona çox uzaqsınız' },
  'you already reported this bus recently, try again in a few minutes': { en: 'You already reported this bus recently, try again in a few minutes', az: 'Bu avtobus haqqında bu yaxınlarda məlumat vermisiniz, bir neçə dəqiqə sonra yenidən cəhd edin' },
  'station is not on this bus\'s route': { en: "This stop is not on the bus's route", az: 'Bu dayanacaq avtobusun marşrutunda deyil' },
  'bus has no route or position yet': { en: 'This bus has no route or position yet', az: 'Bu avtobusun hələ marşrutu və ya mövqeyi yoxdur' },
  'login required': { en: 'Please sign in', az: 'Zəhmət olmasa daxil olun' },
};

function detectLang() {
  try { const s = localStorage.getItem('lang'); if (s === 'az' || s === 'en') return s; } catch (_) {}
  const nav = (navigator.languages || [navigator.language || 'az']).map(l => l.toLowerCase());
  return nav.some(l => l.startsWith('en')) && !nav.some(l => l.startsWith('az')) ? 'en' : 'az';
}

let LANG = detectLang();

function t(key, vars) {
  let s = (I18N[LANG] && I18N[LANG][key]) ?? I18N.en[key] ?? key;
  if (vars) s = s.replace(/\{(\w+)\}/g, (m, k) => (vars[k] ?? m));
  return s;
}

// Translates a server error message when it is a known one.
function tError(msg) {
  const key = String(msg || '').toLowerCase().replace(/^conflict: /, '').trim();
  for (const k of Object.keys(SERVER_ERRORS)) {
    if (key === k || key.endsWith(k)) return SERVER_ERRORS[k][LANG] || SERVER_ERRORS[k].en;
  }
  return msg;
}

function levelLabel(d) { return t('level.' + (d || 'unknown')); }

// Applies translations to static markup: data-i18n (text), data-i18n-placeholder, -title, -aria-label.
function applyI18n(root = document) {
  document.documentElement.lang = LANG;
  root.querySelectorAll('[data-i18n]').forEach(el => { el.textContent = t(el.dataset.i18n); });
  root.querySelectorAll('[data-i18n-placeholder]').forEach(el => { el.placeholder = t(el.dataset.i18nPlaceholder); });
  root.querySelectorAll('[data-i18n-title]').forEach(el => { el.title = t(el.dataset.i18nTitle); });
  root.querySelectorAll('[data-i18n-aria-label]').forEach(el => { el.setAttribute('aria-label', t(el.dataset.i18nAriaLabel)); });
  const titleKey = document.body && document.body.dataset.titleKey;
  if (titleKey) document.title = t(titleKey) + (titleKey === 'app.name' ? '' : ' · ' + t('app.name'));
  document.querySelectorAll('[data-lang]').forEach(b => {
    b.classList.toggle('active', b.dataset.lang === LANG);
    b.setAttribute('aria-pressed', String(b.dataset.lang === LANG));
  });
}

function setLang(lang) {
  if (lang !== 'az' && lang !== 'en') return;
  LANG = lang;
  try { localStorage.setItem('lang', lang); } catch (_) {}
  applyI18n();
  window.dispatchEvent(new CustomEvent('langchange', { detail: lang }));
}

document.addEventListener('click', e => {
  const b = e.target.closest('[data-lang]');
  if (b) setLang(b.dataset.lang);
});
applyI18n();
