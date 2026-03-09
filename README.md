[Русская версия (README.ru.md)](README.ru.md)

# eventbus

Lightweight in-memory publish/subscribe event bus for Go.

## Features

- **Non-blocking publish** — slow subscribers never stall publishers; events are dropped with a warning
- **At-most-once delivery** — designed for reconciliation-driven architectures where missed events trigger state sync
- **Type-based routing** — subscribers receive only events matching their registered type
- **Channel ownership** — subscribers create and close their own channels
- **Thread-safe** — concurrent publish/subscribe/unsubscribe operations are safe
- **Zero dependencies** — only the Go standard library

## Install

```bash
go get github.com/unolink/eventbus
```

## Usage

Define an event:

```go
type OrderCreated struct {
    OrderID string
}

func (e OrderCreated) EventType() string { return "order_created" }
```

Subscribe and publish:

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

## Logger interface

The bus accepts any logger implementing:

```go
type Logger interface {
    Warn(msg string, args ...any)
    Debug(msg string, args ...any)
}
```

Compatible with `slog.Logger` and most structured loggers.

## Design decisions

**Buffered channels only.** `Subscribe` panics on unbuffered channels to prevent deadlocks. Choose buffer size based on expected burst volume.

**Drop over block.** When a subscriber's channel is full, the event is dropped and a warning is logged. This keeps publishers fast and predictable.

**Idempotent unsubscribe.** The function returned by `Subscribe` can be called multiple times safely.

## License

MIT — see [LICENSE](LICENSE).
