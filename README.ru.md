[English version (README.md)](README.md)

# eventbus

Легковесная шина событий publish/subscribe для Go.

## Возможности

- **Неблокирующая публикация** — медленные подписчики не блокируют издателей; события сбрасываются с предупреждением
- **Доставка at-most-once** — для архитектур с reconciliation, где пропущенные события запускают синхронизацию состояния
- **Маршрутизация по типу** — подписчики получают только события своего типа
- **Владение каналами** — подписчики сами создают и закрывают свои каналы
- **Потокобезопасность** — конкурентные publish/subscribe/unsubscribe операции безопасны
- **Без зависимостей** — только стандартная библиотека Go

## Установка

```bash
go get github.com/unolink/eventbus
```

## Использование

Определение события:

```go
type OrderCreated struct {
    OrderID string
}

func (e OrderCreated) EventType() string { return "order_created" }
```

Подписка и публикация:

```go
bus := eventbus.New(logger)

ch := make(chan eventbus.Event, 16)
unsubscribe := bus.Subscribe("order_created", ch)
defer unsubscribe()
defer close(ch)

bus.Publish(OrderCreated{OrderID: "123"})

event := <-ch
order := event.(OrderCreated)
```

## Интерфейс логгера

Шина принимает любой логгер, реализующий:

```go
type Logger interface {
    Warn(msg string, args ...any)
    Debug(msg string, args ...any)
}
```

Совместим с `slog.Logger` и большинством структурированных логгеров.

## Проектные решения

**Только буферизованные каналы.** `Subscribe` паникует при передаче небуферизованного канала для предотвращения дедлоков. Размер буфера выбирается исходя из ожидаемого объёма событий.

**Сброс вместо блокировки.** Когда канал подписчика заполнен, событие сбрасывается с записью предупреждения в лог. Это обеспечивает быстроту и предсказуемость издателей.

**Идемпотентная отписка.** Функция, возвращаемая `Subscribe`, может безопасно вызываться многократно.

## Лицензия

MIT — см. [LICENSE](LICENSE).
