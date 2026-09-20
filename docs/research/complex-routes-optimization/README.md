# Повторная проверка после оптимизации

Те же координаты, даты 15–22 октября 2026, один взрослый и конфигурационные бюджеты: до 240 запросов Яндекса, 30 Google Flights, 20 схем. Лимит подвоза новой версии — 5 вариантов на схему. Сохранённые заявки не пересчитывались, выполнялась ручная команда из текущих исходников. Сравниваются последовательные наблюдения внешних источников, а не один неизменный снимок данных; изменение выдачи нельзя целиком приписывать алгоритму.

| Направление | Кандидатов раньше | Схем теперь | Всего запросов раньше / теперь | Разных пар Flights раньше / теперь |
| --- | ---: | ---: | --- | --- |
| Курск → Лос-Анджелес | 20 | 20 | 211 / 248 | 10 / 30 |
| Воркута → Рейкьявик | 0 | 0 | 171 / 171 | 10 / 10 |
| Йошкар-Ола → Сидней | 5 | 20 | 199 / 260 | 10 / 30 |

Старый счётчик включает разные вокзалы как отдельные кандидаты. Новый — группирует железнодорожный подвоз; разные источники по-прежнему могут сохранять одинаковую физическую цепочку отдельно. Увеличение числа схем не является подтверждением расписаний или наличия билетов.

## Найденные схемы

### Курск → Лос-Анджелес

[Исходный JSON](kursk-la.json). complete=false, limit_reached=true, ошибок источников: 17.

- Domodedovo International Airport (DME) → Cairo International Airport (CAI) → Los Angeles International Airport (LAX); вариантов подвоза поездом: 4.
- Vnukovo International Airport (VKO) → İstanbul Airport (IST) → Los Angeles International Airport (LAX); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Dubai International Airport (DXB) → San Francisco International Airport (SFO) → Hollywood Burbank/Bob Hope Airport (BUR); вариантов подвоза поездом: 4.
- Vnukovo International Airport (VKO) → Dubai International Airport (DXB) → San Francisco International Airport (SFO) → Hollywood Burbank/Bob Hope Airport (BUR); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Dubai International Airport (DXB) → Seattle–Tacoma International Airport (SEA) → Long Beach International Airport (LGB); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Queen Alia International Airport (AMM) → Chicago O'Hare International Airport (ORD) → Los Angeles International Airport (LAX); вариантов подвоза поездом: 4.
- Vnukovo International Airport (VKO) → Dubai International Airport (DXB) → Los Angeles International Airport (LAX); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Cairo International Airport (CAI) → Munich Airport (MUC) → San Francisco International Airport (SFO) → Hollywood Burbank/Bob Hope Airport (BUR); вариантов подвоза поездом: 4.
- Vnukovo International Airport (VKO) → Dubai International Airport (DXB) → Seattle–Tacoma International Airport (SEA) → Hollywood Burbank/Bob Hope Airport (BUR); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Dubai International Airport (DXB) → Los Angeles International Airport (LAX); вариантов подвоза поездом: 4.
- Vnukovo International Airport (VKO) → Dubai International Airport (DXB) → San Francisco International Airport (SFO) → Los Angeles International Airport (LAX); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Cairo International Airport (CAI) → Munich Airport (MUC) → Denver International Airport (DEN) → Hollywood Burbank/Bob Hope Airport (BUR); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Queen Alia International Airport (AMM) → Dallas Fort Worth International Airport (DFW) → Los Angeles International Airport (LAX); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Cairo International Airport (CAI) → Munich Airport (MUC) → Seattle–Tacoma International Airport (SEA) → Hollywood Burbank/Bob Hope Airport (BUR); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Queen Alia International Airport (AMM) → John F. Kennedy International Airport (JFK) → Los Angeles International Airport (LAX); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Cairo International Airport (CAI) → Frankfurt Main Airport (FRA) → San Francisco International Airport (SFO) → Hollywood Burbank/Bob Hope Airport (BUR); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Dubai International Airport (DXB) → San Francisco International Airport (SFO) → Los Angeles International Airport (LAX); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Cairo International Airport (CAI) → Frankfurt Main Airport (FRA) → Seattle–Tacoma International Airport (SEA) → Hollywood Burbank/Bob Hope Airport (BUR); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Dubai International Airport (DXB) → George Bush Intercontinental Airport (IAH) → Los Angeles International Airport (LAX); вариантов подвоза поездом: 4.
- Domodedovo International Airport (DME) → Cairo International Airport (CAI) → Frankfurt Main Airport (FRA) → Denver International Airport (DEN) → Hollywood Burbank/Bob Hope Airport (BUR); вариантов подвоза поездом: 4.

### Воркута → Рейкьявик

[Исходный JSON](vorkuta-reykjavik.json). complete=false, limit_reached=true, ошибок источников: 16.

В пределах этого поиска схемы не найдены.

### Йошкар-Ола → Сидней

[Исходный JSON](yoshkar-sydney.json). complete=false, limit_reached=true, ошибок источников: 13.

- Казань → Абу-Даби → Кингсфорд Смит; подвоз см. в JSON.
- Казань → Пудун → Кингсфорд Смит; подвоз см. в JSON.
- Kazan International Airport (KZN) → Dubai International Airport (DXB) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Казань → Курумоч → Dubai International Airport (DXB) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Казань → Уфа → Dubai International Airport (DXB) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Vnukovo International Airport (VKO) → İstanbul Airport (IST) → Kuala Lumpur International Airport (KUL) → Sydney Kingsford Smith International Airport (SYD); вариантов подвоза поездом: 2.
- Казань → Внуково → İstanbul Airport (IST) → Kuala Lumpur International Airport (KUL) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Казань → Шереметьево → İstanbul Airport (IST) → Kuala Lumpur International Airport (KUL) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Чебоксары → Шереметьево → İstanbul Airport (IST) → Kuala Lumpur International Airport (KUL) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Казань → Домодедово → İstanbul Airport (IST) → Kuala Lumpur International Airport (KUL) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Kazan International Airport (KZN) → Dubai International Airport (DXB) → Melbourne Airport (MEL) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Казань → Курумоч → Dubai International Airport (DXB) → Melbourne Airport (MEL) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Казань → Уфа → Dubai International Airport (DXB) → Melbourne Airport (MEL) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Vnukovo International Airport (VKO) → Dubai International Airport (DXB) → Singapore Changi Airport (SIN) → Sydney Kingsford Smith International Airport (SYD); вариантов подвоза поездом: 2.
- Казань → Внуково → Dubai International Airport (DXB) → Singapore Changi Airport (SIN) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Казань → Шереметьево → Dubai International Airport (DXB) → Singapore Changi Airport (SIN) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Чебоксары → Шереметьево → Dubai International Airport (DXB) → Singapore Changi Airport (SIN) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Казань → Домодедово → Dubai International Airport (DXB) → Singapore Changi Airport (SIN) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.
- Vnukovo International Airport (VKO) → Dubai International Airport (DXB) → Sydney Kingsford Smith International Airport (SYD); вариантов подвоза поездом: 2.
- Казань → Внуково → Dubai International Airport (DXB) → Sydney Kingsford Smith International Airport (SYD); подвоз см. в JSON.

## Выводы и ограничения

- Для Курска прежние 20 записей означали пять авиационных цепочек с четырьмя вокзалами. Теперь вокзалы вложены, а лимит используется на разные схемы, включая путь через VKO → IST → LAX.
- Поиск расширяет число проверенных пар за счёт повторных дат; при большом числе пар до второго прохода дат бюджет может не дойти. Нельзя трактовать результат как покрытие всего диапазона дат.
- Крупные аэропорты получают приоритет, но это эвристика по каталогу. Чередование municipality также не гарантирует проверку всех московских аэропортов: перечень хабов ограничен и не всегда соответствует реальной агломерации.
- Радиус хабов прежний: Воркута по-прежнему требует отдельного механизма обнаружения дальних узлов с подтверждённым подвозом.
- Общий расход Яндекса может вырасти при исследовании большего числа узлов. Оптимизация увеличивает разнообразие при прежних верхних лимитах, но не обещает уменьшения всех запросов.
- Время стыковок, цены, билеты и предполагаемые трансферы не проверены. Redis, самостоятельный перебор новых авиационных цепочек и дальние хабы не входят в эту реализацию.

## Локальные проверки

- Три регрессионных теста воспроизводили старое поведение до правок и проходят после них.
- `make check`: линтер (0 замечаний), buf lint, Go-тесты с race detector и сборка frontend — успешно.
- `go test -race -tags=integration ./services/search/internal/storage -run "TestSavedResultPages|TestRailGroupingBeforePagination|TestNestedRailVariantsRoundTrip" -count=1` — успешно; временные схемы PostgreSQL изолированы от пользовательских данных.
- [Исходные прогоны и команды](../complex-routes-2026-09-18/README.md).
