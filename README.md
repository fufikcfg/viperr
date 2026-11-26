# viperr

Плоский загрузчик конфигурации: окружение процесса, файл `.env` и XML, где имя тега совпадает с ключом
В окружение процесса библиотека ничего не пишет

```bash
go get github.com/fufikcfg/viperr
```

## Быстрый старт

У каждого поля один тег `config`: сначала ключ, затем опции через запятую
Тег `source` необязателен: без него поле берётся из первого источника, где ключ есть

```go
type Config struct {
    Key         string        `config:"APP_KEY" validate:"required"`
    LockTimeout time.Duration `config:"APP_LOCK_TIMEOUT,default=5m"`
    Workers     int           `config:"APP_WORKER_LIMIT,default=4" validate:"positive"`
    Format      string        `config:"LOGGER_FORMAT,default=inline,oneof=inline|console"`
    LogPath     string        `config:"LOGGER_PATH,path,default=logs"`
    Title       string        `config:"TITLE" source:"xml"`
    DB          struct {
        Host string `config:"DB_HOST" validate:"required,notblank,host"`
    }
}
```

`Load` читает каталог приложения и заполняет структуру

```go
var cfg Config
if err := viperr.Load(&cfg, ""); err != nil {
    log.Fatal(err)
}
```

Пустой `dir` означает найти каталог самому
Иначе передайте его явно: `viperr.Load(&cfg, dir)`

Из каталога читаются, если файлы есть:

1) окружение процесса, имя источника `process`
2) `dir/.env`, имя источника `env`
3) `dir/settings.xml`, а если его нет, то `dir/settings/settings.xml`, имя источника `xml`

Отсутствующий файл пропускается, но имя источника всё равно есть: ключей в нём просто нет
Ошибка разбора файла или полей возвращается

## Приоритет

Выше в списке побеждает нижестоящее
Так читается поле без тега `source`
`default` используется, только когда выбранные источники ключ не содержат

```text
окружение процесса  >  .env  >  XML  >  default
```

Ноль это значение
`APP_LOCK_TIMEOUT=0` в XML перебивает `default=5m`, если поле читает XML

Пустой XML тег значением не считается
`<APP_KEY></APP_KEY>` не затирает `APP_KEY` из `.env` и не мешает default

Пустая строка в окружении или в `.env` (`APP_KEY=`) значением считается и блокирует источники ниже

Повтор ключа в одном файле: в XML остаётся последнее непустое, в `.env` остаётся последнее

## Откуда брать поле

Тег `source` фиксирует один источник
Имена, которые ставит `Load`: `process`, `env`, `xml`

```go
Title string `config:"TITLE" source:"xml" validate:"required"`
Note  string `config:"NOTE" source:"env"`
```

`Title` читается только из XML, даже если тот же ключ есть в окружении или в `.env`
`Note` читается только из `.env`
Поле без `source` идёт по приоритету: процесс, затем `.env`, затем XML, затем default

Если в выбранном источнике ключа нет, срабатывает `default`
Нет и default — поле остаётся нулевым, и это ловит тег `validate`
Опечатка в имени, например `source:"xm"`, даёт ошибку: такого источника нет

Свой список для `Decode` помечается через `viperr.Name`

## Тег config

`default=VALUE` подставляется, когда ключа нет
Запятая внутри значения экранируется обратным слэшем, обратный слэш удваивается:

```go
Origins string `config:"CORS_ALLOWED_ORIGINS,default=http://localhost:3000\\,http://localhost:8080"`
```

`path` только для `string`: путь относительно каталога приложения

`noabs` вместе с `path`: «чужой» абсолютный путь остаётся как есть и не перебазируется на каталог приложения
На Windows это путь от корня (`/etc/gateway/routes.d`), на остальных системах это путь с диском (`C:\var\log`)
Файл по такому пути просто не найдётся — честная ошибка вместо странного `<appdir>\etc\...`

`secret` помечает значение секретом: в отчётах `Report` и `Explain` оно маскируется
Ключи, похожие на секрет (`*_KEY`, `*SECRET*`, `*TOKEN*`, `*PASSWORD*` и т.п.), маскируются и без опции

`oneof=A|B` требует точного совпадения с одним из вариантов
Сравнение идёт по строке как она пришла, до разрешения пути

Пробелы вокруг опций допустимы: `` `config:"APP_KEY, default=dev"` ``

Поле с тегом `config` из одного знака минус не читается

Вложенная структура без своего тега обходится, теги читаются с её полей
Указатель на такую структуру создаётся при обходе

Типы полей: `string`, `bool`, целые и беззнаковые int, `time.Duration`, указатели на эти типы
`bool` разбирается как `strconv.ParseBool` (`1`, `t`, `true`, `0`, `f`, `false`)
Длительность разбирается как `time.ParseDuration` (`5m`, `0`, `1s`)

Поле без тега `config` остаётся нулевым, кроме вложенной структуры

## Пути

Поле с `path` собирается относительно каталога, из которого `Load` читал файлы
Абсолютный путь остаётся абсолютным

На Windows путь только со слэшем (`/logs`, `\logs`) считается относительным к каталогу приложения, а `C:\logs` абсолютным
На остальных системах `C:\logs` тоже относительный: префикс диска снимается, остаётся `logs`

Пустая строка сама в путь не превращается
Запасной путь задаётся так: `` `config:"LOGGER_PATH,path,default=logs"` ``

Каталог приложения ищет `ResolveAppDir`: первый из рабочего каталога, каталога исполняемого файла и его родителя, где лежит `.env`, `settings.xml` или `settings/settings.xml`
Свой список маркеров передаётся аргументами: `ResolveAppDir("go.mod", "routes")`
Если ни в одном маркера нет, возвращается рабочий каталог

Отдельный вызов, без загрузки:

```go
abs := viperr.Resolve("logs", appDir)
strict := viperr.ResolveStrict("/etc/gateway/routes.d", appDir) // чужой абсолютный путь как есть
```

## Один ключ без структуры

`Get` читает одиночный ключ по стандартной цепочке (процесс, затем `.env`, затем XML) из каталога `ResolveAppDir`
`GetFrom` делает то же для явного каталога
`ok` говорит, найден ли ключ; с default он возвращается при отсутствии ключа, и `ok` остаётся `false`

```go
v, ok := viperr.Get("HTTP_PORT")
v, ok = viperr.Get("HTTP_PORT", "8080")
key, ok := viperr.Get("GATEWAY_API_KEY") // секрет: прочитать и не логировать
```

## Откуда взялось значение

`DecodeReport` и `LoadReport` работают как `Decode` и `Load`, но возвращают `Report`: по записи на каждое поле с тегом — ключ, источник-победитель (`process`, `env`, `xml`, `default` или `-`), имя Go поля и значение
Секреты в отчёте уже замаскированы (`***`), его безопасно печатать и логировать

```go
report, err := viperr.LoadReport(&cfg, dir)
if err != nil {
    log.Fatal(err)
}
fmt.Print(report)
```

`Explain` делает то же одним вызовом и пишет таблицу в любой writer — готовый `config print` для CLI:

```go
err := viperr.Explain(os.Stdout, &cfg, dir)
```

## Проверка

Ошибки по полям собираются вместе, в тексте есть имя ключа

1) разбор типа: `APP_LOCK_TIMEOUT=abc` или нечисловой int это ошибка
2) набор значений: `oneof` в теге `config`
3) нормализация: метод `Normalize() error` у секции или у корня; секции раньше корня
4) тег `validate`: значения полей после привязки и нормализации
5) своё правило: метод `Validate() error` у корня, вызывается последним

## Тег validate

Второй тег проверяет уже привязанное значение
Вся обязательность живёт здесь: `validate:"required"` значит значение не нулевое
Запятая разделяет правила, `|` это «или» внутри одного правила, `=` передаёт параметр
В ошибках поле называется ключом из `config`: не `SyncLoop`, а `APP_SYNC_LOOP`
Первая неудача по полю останавливает его остальные правила

```go
type Config struct {
    Host     string        `config:"DB_HOST" validate:"required,notblank,host"`
    DBPort   string        `config:"DB_PORT,default=5432" validate:"port"`
    User     string        `config:"DB_USER" validate:"required,notblank,max=63"`
    SyncLoop time.Duration `config:"APP_SYNC_LOOP" validate:"mindur=1s"`
    Workers  int           `config:"APP_WORKER_LIMIT" validate:"positive"`
    Format   string        `config:"LOGGER_FORMAT,default=inline" validate:"oneof_ci=inline console"`
    Addr     string        `config:"APP_SERVER_URL" validate:"required_without=Port,omitempty,listen_addr"`
    Port     string        `config:"APP_SERVER_PORT" validate:"required_without=Addr,omitempty,port"`
}
```

Встроенные: `required`, `omitempty`, `required_without=Field`, `gt`, `gte`, `lt`, `lte`, `min`, `max`, `eq`, `ne`, `oneof`, `port`, `ip`, `ipv4`, `ipv6`, `fqdn`, `hostname_rfc1123`, `dns_rfc1035_label`, `dirpath`, `duration`
Сравнения для чисел по значению, для строк по длине, для `time.Duration` параметр понимает `1s`

Поверх встроенных поставляются: `notblank`, `mindur`, `maxdur`, `listen_addr`, `oneof_ci`, `writable_dir`, `pg_ident`
И алиасы: `positive` это `gt=0`, `nonneg` это `gte=0`, `host` это `hostname_rfc1123|ip|fqdn`, `log_level`, `pg_sslmode`

`required` на число или длительность не нужен: `gt=0` уже отсекает ноль
Директивы ставятся первыми: `required_without=Port,omitempty,listen_addr`

Свои правила передаются в `Load` или `Validate`, опечатка в имени это ошибка на старте
Своя функция заменяет поставочную, заменить встроенный тег можно только с `WithBuiltinOverride`

```go
err := viperr.Load(&cfg, dir, viperr.WithRules(viperr.Rule{
    Tag: "cron_tz",
    Fn: func(fl viperr.FieldLevel) bool {
        _, err := time.LoadLocation(fl.Field().String())
        return err == nil
    },
    Message: "{field} must be a valid IANA timezone",
}))
```

`viperr.Validate(&v, opts...)` проверяет любую структуру теми же правилами без загрузки
`viperr.Errors` это срез `FieldError`, разбирается через `errors.As`

Нормализация это не проверка: trim и lower живут в `Normalize() error`, производные значения тоже там

Метод `Validate() error` остаётся для того, что тегом не выразить

```go
func (c Config) Validate() error {
    if c.Workers < 1 {
        return fmt.Errorf("APP_WORKER_LIMIT must be positive")
    }
    return nil
}
```

## Свои источники

`Decode` заполняет структуру из списка источников
Первый, в котором ключ есть, побеждает, если у поля нет `source`
Дальше то же правило default

```go
xmlSrc, err := viperr.ParseXMLFile("settings.xml")
if err != nil {
    return err
}
err = viperr.Decode(&cfg, viperr.Options{AppDir: appDir},
    viperr.Name(viperr.SourceProcess, viperr.ProcessEnv()),
    viperr.Name(viperr.SourceEnv, viperr.Map{"APP_KEY": "override"}),
    viperr.Name(viperr.SourceXML, xmlSrc),
)
```

`ParseEnv` и `ParseEnvFile` читают `KEY=VALUE`
Пустая строка и строка с `#` в начале пропускаются, ключ и значение обрезаются по краям
Кавычки, `export`, комментарий в конце строки и многострочные значения не разбираются

`ParseXML` и `ParseXMLFile` берут локальное имя каждого тега
Вложенность только группирует теги, путь из имён не строится
Пустой текст тега в карту не попадает

Свой источник это `Lookup`:

```go
type Source interface {
    Lookup(key string) (value string, ok bool)
}
```

`ok == false` значит ключа нет
`ok == true` и пустая строка значат, что ключ есть

`viperr.Name("xml", src)` даёт этому источнику имя для тега `source`
Без имени поле с `source` его не увидит
Источник без имени участвует только в общем приоритете

`Options.AppDir` это база для полей `path`
Пустой `AppDir` оставляет относительный путь относительным

## Устройство пакета

Публичный API живёт в корневом пакете `viperr`
Реализация разнесена по внутренним пакетам в `internal/`: `bind` (привязка и отчёты), `tag` (разбор тега `config`), `source` (источники), `resolve` (пути и каталог приложения), `dotenv` и `xmlconf` (парсеры файлов), `validate` (движок тега `validate`)
Go toolchain разрешает импорт `internal/...` только коду внутри этого модуля: снаружи доступен лишь корневой пакет
