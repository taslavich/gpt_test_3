# RTB rules: единый набор тестов и 100% покрытия (proj255)

Патч применяется непосредственно к чистому proj255.zip из корня RTB.

## Результат

Все тесты каталога `internal/services/adv/rtbrules` находятся в одном файле
`rtbrules_test.go`. Файлы `store_test.go`, `main_test.go`, `main2_test.go` удалены
после переноса. Все 63 исходные функции тестов/помощников, включая 42 исходных
Test-функции, сохранены с неизменёнными AST тел: это проверено отдельным скриптом.
Новые тесты добавлены в тот же файл.

Реальный запуск Go 1.25.11 с `-race`, `-count=1`, `-covermode=atomic`:

- 100,0% покрытия операторов пакета: 1020 из 1020, ноль непокрытых блоков;
- каждая функция в `go tool cover -func` имеет 100,0%; файлы из покрытия
  не исключались, счётчики не подменялись;
- 56 Test-функций и один Fuzz-target; 181 успешный результат с учётом
  вложенных тестов/seed-кейсов, ноль пропусков;
- дополнительно 17 844 выполнения короткого fuzz-прогона (5 секунд заданного
  бюджета, два worker) без найденных сбоев.

## Проверяемые сценарии

| Область | Проверки |
|---|---|
| filters | whitelist/blacklist/exists, отсутствующее/null/пустое поле, скаляр/массив, независимые условия, точная причина отказа |
| Числа | `>`, `>=`, `<`, `<=`: ниже/равно/выше порога; отсутствие значения, неверный тип, переполнение, неизвестная операция |
| Native | JSON-path, индексы/asset ID, точные значения и размеры, пороги, строка/число/bool, неверный JSON/тип, trailing JSON, пустой/неверный selector |
| Native-кеш | вход с реальным protobuf, одно декодирование на request/impression, повторное использование ошибок, изоляция разных кампаний и impressions |
| add | отсутствующие/существующие protobuf-поля, сохранение присутствующих нулей/пустых значений, создание родителей, JSON postfix, общие родители и конфликты |
| replace | set/set_or_add, countries/iso, hostname/full_page, copy_to с отсутствующим и заполненным назначением, независимые impressions |
| delete | только protobuf pointers, существующий/отсутствующий родитель, неизменность оригинала, конфликты с другими модулями |
| rename | последний этап, допустимое конечное имя, запрет неизвестных/конфликтующих путей |
| Группы | UUID через запятую, группировка полей, ALL + campaign, сохранение остальных правил, поздняя запись при совпадении module/path, стабильность load/reload/PUT |
| Конфигурация | неизвестные ключи/типы/модули, null, дубли JSON-ключей, неверные UUID, смешанные режимы, неверные source/copy_to/postfix |
| Хранилище | исходные JSON-байты без перезаписи, Saved/Memory, глубокая копия, последний валидный snapshot при ошибках чтения/валидации/записи |
| I/O | mkdir/create/write/sync/close/rename failures, возврат исходной ошибки, закрытие и удаление временного файла, сохранность прежнего файла |
| Конкурентность | чтение/Check/Apply параллельно с обновлениями, отсутствие мутации входного protobuf и опубликованных snapshots |
| ADV | настоящие preflight + HTTP fanout: только подходящие impressions; malformed Native отдельно от mismatch; другая кампания и исходный request не затронуты |
| HTTP управления | GET исходных байтов/debug map, PUT успешный/невалидный/слишком большой, 503 без Store, 500 при I/O, сохранение последнего валидного snapshot |

## Минимальные изменения рабочего кода

Новые режимы, поля, ENV и бизнес-правила не добавлены. Карта
`cmd/adv/adv_rtb_rules_map.json` не изменена.

1. `store.go`: исправлен согласованный приоритет «позднее правило заменяет
   раннее при совпадении campaign/module/path». Вместо случайного обхода Go map
   ключи читаются в порядке JSON. Регрессионный тест падал до исправления;
   проверяются оба порядка, повторные загрузки, Reload и UpdateJSON.
   ALL остаётся базой, campaign-specific имеет приоритет независимо от позиции.
2. `atomicWriteWithTemp` принимает создатель временного файла через параметр.
   Рабочий вызов использует тот же `os.CreateTemp`; порядок write → sync → close
   → rename и directory sync сохранён. Это даёт тестам управляемые отказы I/O
   без process-global hooks и без изменений поведения успешной записи.
3. JSON-этап `ApplyToRequest` вынесен в приватный `applyEncodedRules` для прямой
   проверки повреждённого закодированного входа; последовательность операций
   и результат прежние.
4. Убраны только недостижимые дубли проверок: `mode != countries` при ненулевом
   countries невозможен после присваивания mode; rename-source уже проверяется
   validateSourceField; RawMessage внешнего валидного объекта не содержит второго
   JSON-документа; Decoder.Token возвращает строковые object keys либо ошибку.
   Отклонение невалидных конфигураций сохранено и проверено отрицательными кейсами.

В существующих интеграционных тестах ADV/web исправлены устаревшие fixtures и
ожидания: `imp.ext` + `postfix_field` вместо неподдерживаемого `imp.ext.type`,
отсутствие удалённого `formats`, действующая семантика copy_to (копирование
итогового source в destination, включая заполненное destination). Эти тесты
падали и на исходном proj255; рабочая логика copy_to/GEO/форматов не менялась.
Все тесты внутри самого пакета rtbrules сохранены без переписывания ожиданий.

Publisher, финансовые расчёты, аукцион, percenters, маршрутизация, loaders,
продукционная логика ADV/service и ADV/web, protobuf и зависимости не изменены.

## Применение и повторение проверок

```bash
cd ~/gpt_test_3
git apply ~/Downloads/proj255_rtbrules_tests_100pct.patch

go test -race -count=1 -covermode=atomic \
  -coverprofile=rtbrules.cover ./internal/services/adv/rtbrules
go tool cover -func=rtbrules.cover

go test -race -count=1 ./internal/services/adv/web
go test -race -count=1 ./internal/services/adv/service \
  -run '^(TestRTBRules|Test54Ads|TestCallRTBCampaign|TestBidFly)'

go test ./internal/services/adv/rtbrules -run '^$' \
  -fuzz '^FuzzRulesJSONPreservesLastValidSnapshot$' -fuzztime=5s -parallel=2

go vet ./internal/services/adv/rtbrules \
  ./internal/services/adv/web ./internal/services/adv/service
mkdir -p bin
go build -o bin/adv ./cmd/adv
```

Перечисленные проверки и сборка ADV реально выполнены успешно. Выполнены также
`gofmt`, `git diff --check`, `git apply --check`, реальное применение на новой
распаковке proj255 и побайтное сравнение результата с проверенным деревом.
Полный `go test ./...` и production-нагрузка не запускались. Миграций БД нет.
Покрытие относится к Go statements пакета rtbrules, а не ко всему ADV.
