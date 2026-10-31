# Ağıllı Şəhər Nəqliyyatı (Smart City Transportation)

Bakı üçün real vaxtda avtobus izləmə və sıxlığın idarə olunması sistemi. OpenStreetMap xəritəsi üzərində iki panel var:

- **Sərnişin paneli** (`/`): avtobus və marşrut axtarışı, avtobusların canlı mövqeyi, real yollarla çəkilmiş marşrut, istənilən dayanacağa gəliş vaxtı (ETA), avtobusun sıxlığı, sıxlıq barədə məlumat vermək, məkanı paylaşmaq və **test məkanı** (xəritədə nişan qoyaraq olduğunuz yeri simulyasiya etmək).
- **Operator paneli** (`/operator`): istifadəçilərin canlı istilik xəritəsi (heatmap), avtobus / marşrut / dayanacaqların idarə olunması, sıxlıq anomaliyaları, yönləndirmə təklifləri, əl ilə təsdiq və müddət seçimi, brauzer bildirişləri.

Texnologiyalar: **Go 1.24**, **PostgreSQL 16**, MVC, N-qatlı arxitektura (PL / BL / DL), Leaflet + OpenStreetMap, WebSocket. İnterfeys **Azərbaycan və ingilis** dillərində, **işıqlı və qaranlıq** temalarla işləyir.

> İngiliscə versiya: [README.en.md](README.en.md) · Texniki sənəd: [texniki_sened.txt](texniki_sened.txt) · Aİ bəyənnaməsi: [AI_declaration.md](AI_declaration.md)

---

## 1. Layihəni lokal mühitdə başlatmaq

### Variant A: Docker ilə (tövsiyə olunur)

Tələblər: **Docker** və **Docker Compose** (Docker Desktop kifayətdir).

1. Repozitoriyanı klonlayın:
   ```bash
   git clone https://github.com/ruslan-sadigov/digital-challange-semi-final.git
   cd digital-challange-semi-final
   ```
2. Sistemi işə salın (verilənlər bazası + tətbiq):
   ```bash
   docker compose up --build
   ```
3. Brauzerdə açın:
   - Sərnişin paneli: http://localhost:8080 (qonaq sessiyası avtomatik açılır)
   - Operator paneli: http://localhost:8080/operator/login
     - E-poçt: `operator@smartcity.az`
     - Şifrə: `operator123`
4. Dayandırmaq üçün `Ctrl+C`, bazanı da silmək üçün:
   ```bash
   docker compose down -v
   ```

Docker Compose tətbiqi **demo rejimində** işə salır: simulyator 8 real Bakı xəttində 22 avtobusu real yollarla hərəkət etdirir, təxminən 120 sərnişin və 80 piyada yaradır. **65** nömrəli xətt qəsdən sıx saxlanılır, ona görə də təxminən 3 dəqiqə ərzində anomaliya, yönləndirmə təklifləri və təsdiq axını görünür.

### Variant B: Docker olmadan (lokal mühit)

Tələblər:
- **Go 1.24** və ya daha yeni
- **PostgreSQL 16** (lokal quraşdırılmış)

1. Repozitoriyanı klonlayın və qovluğa keçin (yuxarıdakı kimi).
2. Verilənlər bazasını yaradın:
   ```bash
   createdb -U postgres smartcity
   ```
3. Tətbiqi demo rejimində işə salın (cədvəllər və real Bakı şəbəkəsi ilk başlanğıcda avtomatik yaradılır):
   ```bash
   DEMO=true DATABASE_URL="postgres://postgres:postgres@localhost:5432/smartcity?sslmode=disable" go run ./cmd/server
   ```
   Real tələblərdəki vaxtlarla (10 dəqiqəlik anomaliya qaydası) işə salmaq üçün `DEMO=true` hissəsini çıxarın.
4. Brauzerdə http://localhost:8080 ünvanını açın.

PostgreSQL-i yalnız Docker-də, tətbiqi isə lokal işə salmaq da olar:
```bash
make db
```
```bash
make demo
```

### Testlər

Unit testlər (verilənlər bazası lazım deyil):
```bash
make test
```
İnteqrasiya testləri (PostgreSQL və `smartcity_test` bazası lazımdır, `make db` onu yaradır):
```bash
make test-integration
```
Ümumi əhatə (coverage) hesabatı:
```bash
make cover
```

### Əsas konfiqurasiya

| Dəyişən | Standart dəyər | İzah |
| --- | --- | --- |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/smartcity?sslmode=disable` | PostgreSQL bağlantısı |
| `ADDR` | `:8080` | HTTP ünvanı |
| `DEMO` | `false` | Sürətli vaxtlar (2 dəq anomaliya, 10 san yoxlama) |
| `SIMULATE` / `SEED` | `true` | Simulyator və real Bakı şəbəkəsinin yüklənməsi |
| `SESSION_SECRET` | `dev-secret-change-me` | Sessiya kukilərinin imza açarı (istehsalda dəyişin) |
| `OPERATOR_EMAIL` / `OPERATOR_PASSWORD` | `operator@smartcity.az` / `operator123` | İlk başlanğıcda yaradılan operator |
| `DEVICE_API_KEY` | `dev-device-key` | Avtobus GPS cihazları üçün açar |
| `OSRM_URL` | `https://router.project-osrm.org` | Real yol marşrutlaması; `off` = düz xətt |
| `ANOMALY_THRESHOLD` | `10m` (demo: `2m`) | Marşrut neçə müddət yüksək sıxlıqda qalanda anomaliya sayılır |
| `CHECK_INTERVAL` / `HEATMAP_INTERVAL` | `60s` / `30s` | Anomaliya yoxlaması və istilik xəritəsi yenilənməsi |
| `USER_SAMPLE_INTERVAL` | `2m` | İstifadəçi məkanının göndərilmə intervalı |

---

## 2. Sistem arxitekturası (Frontend → Backend → DB / API axını)

```
┌──────────────────────────── Brauzer (Frontend) ────────────────────────────┐
│  Sərnişin paneli        Operator paneli        Giriş / Qeydiyyat            │
│  HTML şablonları (Go) + JavaScript + Leaflet (OpenStreetMap plitələri)      │
│  i18n (az/en), işıqlı/qaranlıq tema                                         │
└──────────────┬───────────────────────────────┬─────────────────────────────┘
               │ HTTPS: REST API (JSON)        │ WebSocket (/ws): canlı yeniləmələr
               ▼                               ▼
┌──────────────────────────── Backend (Go, bir xidmət) ──────────────────────┐
│ PL  Təqdimat qatı: router, controller-lər (MVC "C"), view-lar (MVC "V"),     │
│     middleware (sessiya, rol yoxlaması, cihaz açarı), WebSocket hub         │
│ BL  Biznes qatı: auth, fleet (avtobus/marşrut/dayanacaq), location,         │
│     density (sıxlıq), ETA, heatmap, anomaly + reroute (yönləndirmə)         │
│ DL  Məlumat qatı: repozitoriya interfeysləri, PostgreSQL (pgx), OSRM klienti│
│ Worker-lər: anomaliya yoxlaması, heatmap yayımı, simulyator, seed           │
└──────────────┬───────────────────────────────┬─────────────────────────────┘
               │ SQL (pgx)                     │ HTTP (yalnız yol marşrutlaması)
               ▼                               ▼
     ┌────────────────────┐          ┌──────────────────────────┐
     │  PostgreSQL 16      │          │  OSRM (OpenStreetMap      │
     │  istifadəçilər,     │          │  əsaslı yol marşrutları)  │
     │  avtobuslar, GPS,   │          └──────────────────────────┘
     │  sıxlıq, anomaliya  │
     └────────────────────┘
   Avtobus GPS cihazları ──HTTP POST /api/device/buses/{id}/location──▶ Backend
```

### Qatlar

- **Frontend**: Go `html/template` ilə serverdə render olunan səhifələr (MVC "View") və onların üzərində işləyən JavaScript. Xəritə Leaflet ilə OpenStreetMap plitələri üzərində çəkilir. Brauzer məlumatları REST API ilə alır və göndərir, canlı yeniləmələri isə bir WebSocket bağlantısı ilə qəbul edir.
- **PL (Presentation Layer)**: HTTP router, controller-lər, sessiya və rol yoxlayan middleware, WebSocket hub. Controller-lər yalnız sorğunu oxuyur, BL servisini çağırır və cavabı JSON və ya HTML kimi qaytarır.
- **BL (Business Layer)**: bütün qaydalar buradadır: sıxlığın hesablanması, istifadəçi məkanlarından sərnişin sayının təxmini, 10 dəqiqəlik anomaliya qaydası, yönləndirmə təklifləri, yol boyunca ETA. BL yalnız DL interfeyslərindən asılıdır, ona görə də unit testlərlə yoxlanılır.
- **DL (Data Layer)**: repozitoriya interfeysləri və onların PostgreSQL (pgx) tətbiqi; OSRM üçün yol marşrutu klienti.
- **Worker-lər**: anomaliya yoxlaması (hər 60 san), istilik xəritəsinin yayımı (hər 30 san), demo simulyatoru, real şəbəkənin yüklənməsi.

### Əsas axınlar

1. **Avtobus mövqeyi**: GPS cihazı (və ya simulyator) → `POST /api/device/buses/{id}/location` → BL mövqeyi və hərəkət istiqamətini saxlayır → DL `bus_location` cədvəlinə yazır → WebSocket hub `bus_location` mesajını bütün xəritələrə göndərir → markerlər yerini və istiqamət üçbucağını dəyişir.
2. **Sıxlıq**: sərnişin "Az / Orta / Yüksək" düyməsini basır → `POST /api/buses/{id}/density` → BL tezlik və məsafə qaydalarını yoxlayır, məlumatı `bus_status` cədvəlinə yazır → sıxlıq yenidən hesablanır (son məlumatlar + məkanı paylaşan istifadəçilərdən təxmini sərnişin sayı) → WebSocket ilə bütün panellərə göndərilir.
3. **Anomaliya və yönləndirmə**: worker hər dəqiqə marşrut sıxlığını hesablayır → marşrut ardıcıl 10 dəqiqə "yüksək" qalırsa anomaliya açılır, az sıxlıqlı başqa marşrutlardan avtobus təklif edilir və operatora bildiriş gedir → operator təklifi təsdiqləyir və müddəti seçir → avtobus real yollarla yeni xəttə gedir və yalnız qoşulduqdan sonra həmin xəttin sıxlığına daxil edilir.
4. **İstilik xəritəsi**: brauzer məkanı hər 2 dəqiqədən bir `POST /api/locations` ilə göndərir → worker son məlumatları təxminən 100 m-lik xanalara toplayır → `operator:heatmap` kanalı ilə yalnız operatorlara göndərir (fərdi nöqtələr serverdən çıxmır).

### Əsas API-lər

| Metod | Yol | Rol | Təyinat |
| --- | --- | --- | --- |
| POST | `/api/auth/register`, `/api/auth/login`, `/api/auth/operator/login`, `/api/auth/guest` | hamı | Qeydiyyat, sərnişin girişi, operator girişi, qonaq sessiyası |
| GET | `/api/buses?q=`, `/api/buses/{id}`, `/api/buses/{id}/eta?station_id=` | hamı | Axtarış, avtobus məlumatı, gəliş vaxtı |
| GET | `/api/routes`, `/api/routes/{id}`, `/api/routes/density`, `/api/stations`, `/api/stations/near` | hamı | Marşrutlar (yol xətti ilə), sıxlıq, dayanacaqlar |
| POST | `/api/buses/{id}/density`, `/api/locations` | qonaq, sərnişin | Sıxlıq bildirmək, məkan göndərmək |
| POST | `/api/device/buses/{id}/location` | GPS cihazı (açar) | Avtobus mövqeyi |
| GET/POST/PUT/DELETE | `/api/operator/...` | operator | Heatmap, CRUD, anomaliyalar, təsdiq / rədd / bitirmə |
| WS | `/ws` | sessiya | Kanallar: `buses`, `operator:heatmap`, `operator:alerts` |

### Verilənlər bazası

Əsas cədvəllər: `app_user`, `station`, `route`, `route_stop`, `bus`, `bus_location`, `user_location`, `bus_status` (sıxlıq məlumatları), `route_density_state`, `density_anomaly`, `route_override` (yönləndirmələr), `app_meta`. Miqrasiyalar `migrations/` qovluğundadır və tətbiq başlayanda avtomatik tətbiq olunur.

### Real xəritə məlumatları

Şəbəkə OpenStreetMap-dən götürülmüş **real Bakı avtobus xətləridir**: 2, 3, 10, 13, 65, 85, 88 və 93 nömrəli xətlər, **374 real dayanacaq** (real adları ilə) və hər xəttin getdiyi dəqiq yollar. Məlumatlar `go run ./cmd/seedgen` ilə OSM API-dən yüklənib `internal/worker/seeddata/baku.json` faylına yazılır və tətbiqə daxil edilir, buna görə tətbiq internetsiz də işləyir.

## Ekran görüntüləri

![Sərnişin paneli](docs/screenshots/passenger.png)
![Qaranlıq tema](docs/screenshots/passenger-dark.png)
![Avtobus markerləri](docs/screenshots/bus-markers.png)
![Operator paneli](docs/screenshots/operator-alerts.png)

Xəritə məlumatları © OpenStreetMap iştirakçıları (ODbL).
