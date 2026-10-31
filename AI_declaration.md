# Süni intellekt (Aİ) istifadəsi haqqında bəyənnamə (AI declaration)

Bu sənəd "Ağıllı Şəhər Nəqliyyatı (Smart City Transportation)" layihəsində kodun yazılması, testlər və arxitektura seçimində hansı Aİ alətlərindən, necə və hansı promptlarla istifadə edildiyini açıq şəkildə bəyan edir.

## 1. İstifadə olunan Aİ alətləri

| Alət | İstehsalçı | Harada istifadə olunub |
| --- | --- | --- |
| **Claude Code** (bulud sessiyası, agent rejimi) | Anthropic | Dizayn sənədi, arxitektura, bütün backend və frontend kodu, verilənlər bazası sxemi, unit və inteqrasiya testləri, real xəritə məlumatlarının generatoru, dizayn sistemi, tərcümələr, sənədlər, Git commit-ləri |
| **Claude Docs** (Claude Code daxilində sənəd aləti) | Anthropic | İlkin dizayn və icra planı sənədinin yaradılması |

Layihə üzərində işin hamısı bir Claude Code sessiyasında aparılıb. **Cursor, GitHub Copilot, ChatGPT və digər Aİ alətlərindən bu sessiyada istifadə olunmayıb.** Komanda bu sessiyadan kənarda başqa Aİ alətindən istifadə edibsə, onu yuxarıdakı cədvələ əlavə etməlidir.

Aİ olmayan, lakin istifadə olunmuş xarici mənbələr:
- **OpenStreetMap** (OSM API, Nominatim): real avtobus xətləri, dayanacaqlar və yollar (ODbL lisenziyası).
- **OSRM** (OpenStreetMap əsaslı marşrut xidməti): operatorun yaratdığı marşrutlar və yönləndirilən avtobuslar üçün real yol xətti.
- **Playwright** (brauzer avtomatlaşdırması): Aİ agenti interfeysi real brauzerdə yoxlamaq və ekran görüntüləri çəkmək üçün istifadə edib.

## 2. Aİ-dən necə istifadə olunub

Komanda tələbləri yazıb, açıq sualları cavablandırıb və hər mərhələdə nəticəni yoxlayaraq yeni tələblər verib. Claude Code isə agent kimi işləyib: kodu yazıb, terminalda əmrləri işə salıb, testləri icra edib, tətbiqi işə salıb, brauzerdə yoxlayıb və nəticəni GitHub-a göndərib.

| Sahə | Aİ-nin rolu | Komandanın rolu |
| --- | --- | --- |
| **Arxitektura** | N-qatlı (PL/BL/DL) MVC strukturunu, verilənlər bazası sxemini və təklif olunan cədvəllərdəki düzəlişləri (məs. `BusRoute` cədvəlinin marşrut + sıralı dayanacaqlara bölünməsi), real vaxt üçün WebSocket-i təklif edib; 14 açıq sualı standart cavablarla siyahılayıb | Texnologiya yığınını (Go, PostgreSQL, MVC, N-qatlı) müəyyən edib, açıq sualları cavablandırıb (əl ilə təsdiq, 10 dəqiqəlik qaydanın mənası, məkandan sərnişin sayı, arxa planda məkan), müsabiqə üçün həcmin azaldılmasını qəbul edib |
| **Kod** | Bütün Go kodu (domain, BL servisləri, repozitoriyalar, controller-lər, WebSocket hub, worker-lər, simulyator), SQL miqrasiyaları, HTML şablonları, JavaScript, CSS | Tələbləri və prioritetləri müəyyən edib, nəticəni yoxlayıb, düzəliş tələb edib |
| **Testlər** | 62 test funksiyası: biznes qaydaları üçün unit testlər (sıxlıq, anomaliya detektoru, yönləndirmə təklifçisi, ETA, istiqamət), middleware və WebSocket testləri, real PostgreSQL üzərində 11 inteqrasiya testi | Test tələbini qoyub |
| **Məlumat** | OSM API-dən real Bakı avtobus xətlərini yükləyən generator (`cmd/seedgen`) | Real dayanacaq və xətlərin istifadə olunmasını tələb edib |
| **Dizayn** | "Essential" dizayn sistemini tətbiq edib, qaranlıq tema və Azərbaycan / ingilis tərcümələrini hazırlayıb | Dizayn sistemini seçib, dil və tema tələbini qoyub |
| **Sənədlər** | Dizayn sənədi, README (az / en), texniki sənəd, bu bəyənnamə | Sənəd tələblərini müəyyən edib |

### Aİ-nin özü tapıb düzəltdiyi məntiq xətaları (yoxlama zamanı)

Aİ işlədiyi müddətdə tətbiqi real işə salıb və aşağıdakı problemləri özü aşkar edib düzəldib; hər biri üçün test əlavə olunub:
- Məkanını paylaşan istifadəçi tapılmayan avtobus "boş" sayılırdı və real sıxlıq məlumatlarını aşağı salırdı; indi bu "məlumat yoxdur" kimi qəbul olunur.
- Anomaliya həll olunanda yönləndirmə avtomatik bitirdi, avtobus geri qayıdırdı və marşrut yenidən sıxlaşırdı (dövrə); indi yönləndirmə operator bitirənə və ya müddət bitənə qədər davam edir.
- Təsdiqlənmiş avtobus hələ yeni xəttə çatmamış həmin xəttin sıxlığına daxil edilirdi; indi yalnız qoşulduqdan sonra sayılır.
- Gediş və dönüş yolları yanaşı olan yerlərdə ETA səhv istiqamətə görə hesablana bilərdi; indi dayanacaqlar hərəkət sırası ilə, avtobus isə istiqamətinə görə yerləşdirilir.
- Seçilmiş dayanacaqlardan biri avtobusun yolundan 87 m kənarda idi; generator belə dayanacaqları artıq daxil etmir.

## 3. İstifadə olunan promptlar

Aşağıda komandanın Claude Code-a verdiyi promptlar ardıcıllıqla göstərilib (orijinal dildə; uzun hissələr qısaldılıb, `[...]` ilə işarələnib).

1. **İlkin tələb və arxitektura sorğusu**
   > App Title: Agilli Mobillik ve Neqliyyat. There will be two user panel. Each will see maps (from openmap). User side: Can search buses, see their locations and their ETA, their route and density of bus. User can report density of bus. Operator side: Operator can see users as heatmap in openmap in real-time. Operator can add/edit buses, their route. System must suggest from other low-density buses' route [...] if one buses' route's density is high for more than 10 minutes. Send push notification anomaly [...] DB structure: [User, UserLocation, BusStatus, Bus, BusLocation, Station, BusRoute] [...] Tech Stack: Go, PostgreSQL, MVC project. App architecture: N-Layered (BL, DL, PL). Write unit and integration tests for the system. Give us a documentation for how you will implement and general design of the app. IF there is anything ambiguous or needs to be checked again, let me know.

2. **Dizayn sənədinə komandanın düzəlişləri** (sənəddə: sərnişin sayı istifadəçi məkanlarından da hesablanmalıdır; məkan arxa planda alınmalıdır; müsabiqə bir neçə saatdır) və prompt:
   > we have changed the document, check it and give an output if there is still something needs to be checked before implementing

3. **Qərarlar və icraya başlamaq**
   > Reroute approval: it is manually approved. "High for 10 minutes" [...] means the route density is in high status for more than 10 minutes. each route should have its density which is dynamic based on real data. other questions is okay with your default suggestions. start building it

4. **GitHub**
   > add a remote repo for this repo ruslan-sadigov/digital-challange-semi-final

   > push it to main

5. **Real yollar, ayrı giriş, test məkanı**
   > take bus routes as real road routes and take stations as real locations on map. Split user and operator and user sign in tab and also add registration tab, which should be different than login. To test ETA, add a test button to point (which will simulate our location) beside of sharing current location

6. **Dizayn sistemi**
   > /essential-design apply this design skill to the project

7. **Real OSM xətləri, əl ilə müddət, adın dəyişməsi, real vaxtda yönləndirmə**
   > bus routes still does not follow real roads on map. also bus stations are underground stations currently, fetch the real station data from openstreetmap with real bus numbers. make route change interval by operator manual not automatic. also change domain name from agilli to smart city transportation. when we change bus route it shouldnt change immidiately to new route, it should go in real time with real roads.

8. **Markerlər, dillər, tema**
   > bus numbers should be available in icons and show bus direction as mini triangle icon, add multi language (az, en), also add dark/light theme

9. **Sənədlər**
   > write readme in azerbaihani [...] Aİ_declaration.md faylı yarat [...] texniki_sened.txt faylı yarat [...]

## 4. Yoxlama və məsuliyyət

- Aİ-nin yazdığı kod hər mərhələdə avtomatik testlərlə (`go vet`, unit və inteqrasiya testləri) və real brauzerdə (Playwright) yoxlanılıb; testlər keçmədən dəyişikliklər göndərilməyib.
- Azərbaycan dilindəki interfeys mətnləri Aİ tərəfindən tərcümə olunub; təqdimatdan əvvəl ana dilli şəxs tərəfindən yoxlanılması tövsiyə olunur.
- Son məhsula, onun düzgünlüyünə və təqdimatına görə məsuliyyəti komanda daşıyır.
