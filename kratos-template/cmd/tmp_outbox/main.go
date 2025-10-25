package main

import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/bionicotaku/lingo-utils/outbox/store"
    "github.com/go-kratos/kratos/v2/log"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
)

func main() {
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" {
        panic("DATABASE_URL not set")
    }
    ctx := context.Background()
    pool, err := pgxpool.New(ctx, dsn)
    if err != nil {
        panic(err)
    }
    defer pool.Close()

    if _, err := pool.Exec(ctx, "SET search_path TO catalog,public"); err != nil {
        panic(err)
    }

    repo := store.NewRepository(pool, log.NewStdLogger(os.Stdout))
    msg := store.Message{
        EventID:       uuid.New(),
        AggregateType: "video",
        AggregateID:   uuid.New(),
        EventType:     "catalog.video.created",
        Payload:       []byte(`{"ok":true}`),
        Headers:       map[string]string{"schema_version": "v1"},
        AvailableAt:   time.Now().UTC(),
    }
    if err := repo.Enqueue(ctx, nil, msg); err != nil {
        fmt.Println("enqueue error:", err)
        return
    }

    fmt.Println("enqueue success")
}

