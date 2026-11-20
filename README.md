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
    Key         string        `config:"APP_KEY,required"`
    LockTimeout time.Duration `config:"APP_LOCK_TIMEOUT,default=5m"`
    Workers     int           `config:"APP_WORKER_LIMIT,default=4"`
    Format      string        `config:"LOGGER_FORMAT,default=inline,oneof=inline|console"`
    LogPath     string        `config:"LOGGER_PATH,path,default=logs"`
    Title       string        `config:"TITLE" source:"xml"`
    DB          struct {
        Host string `config:"DB_HOST,required"`
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
Title string `config:"TITLE,required" source:"xml"`
Note  string `config:"NOTE" source:"env"`
```

`Title` читается только из XML, даже если тот же ключ есть в окружении или в `.env`
`Note` читается только из `.env`
Поле без `source` идёт по приоритету: процесс, затем `.env`, затем XML, затем default

Если в выбранном источнике ключа нет, срабатывает `default` или `required`
Опечатка в имени, например `source:"xm"`, даёт ошибку: такого источника нет

Свой список для `Decode` помечается через `viperr.Name`

## Тег config

`required` означает, что ключа нет в выбранных источниках и нет `default`
Вместе с `default` он не конфликтует: default закрывает отсутствие ключа
Это не проверка «значение не ноль»

`default=VALUE` подставляется, когда ключа нет

`path` только для `string`: путь относительно каталога приложения

`oneof=A|B` требует точного совпадения с одним из вариантов
Сравнение идёт по строке как она пришла, до разрешения пути

Пробелы вокруг опций допустимы: `` `config:"APP_KEY, required"` ``

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
Если ни в одном маркера нет, возвращается рабочий каталог

Отдельный вызов, без загрузки:

```go
abs := viperr.Resolve("logs", appDir)
```

## Проверка

Ошибки по полям собираются вместе, в тексте есть имя ключа

1) разбор типа: `APP_LOCK_TIMEOUT=abc` или нечисловой int это ошибка
2) присутствие: `required` без значения и без default
3) набор значений: `oneof`
4) своё правило: если у значения есть метод `Validate() error`, `Decode` вызывает его после заполнения полей

Так проверяют то, что не выразить тегом, например лимит воркеров больше нуля
Общий порог «больше нуля» на все поля не используется: он отверг бы законный `APP_LOCK_TIMEOUT=0`

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
