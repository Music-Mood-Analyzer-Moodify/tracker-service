package messaging

import (
	"context"
	"encoding/json"
	"log/slog"
	"tracker-service/internal/model"

	amqp "github.com/rabbitmq/amqp091-go"
)

func SetUpMessaging(connectionString string) (*amqp.Connection, *amqp.Channel, amqp.Queue, error) {
    conn, err := amqp.Dial(connectionString)

    if err != nil {
        slog.Error("Failed to connect to RabbitMQ", "error", err)
        return nil, nil, amqp.Queue{}, err
    }

    ch, err := conn.Channel()
    if err != nil {
        slog.Error("Failed to open a channel", "error", err)
        return nil, nil, amqp.Queue{}, err
    }

    qSendSong, err := ch.QueueDeclare(
        "send_song_queue",
        false,
        false,
        false,
        false,
        nil,
    )
    if err != nil {
        slog.Error("Failed to declare send_song_queue", "error", err)
        return nil, nil, amqp.Queue{}, err
    }

    return conn, ch, qSendSong, nil
}

func SendJsonMessage(ch *amqp.Channel, q amqp.Queue, song model.Song) {
    ctx := context.Background()

    jsonBytes, err := json.Marshal(song)
    if err != nil {
        slog.Error("Failed to marshal song to JSON", "error", err)
        return
    }

    slog.InfoContext(ctx, "Producing message: "+string(jsonBytes))
    err = ch.PublishWithContext(
        ctx,
        "",
        q.Name,
        false,
        false,
        amqp.Publishing{
            ContentType: "application/json",
            DeliveryMode: amqp.Persistent,
            Body:        jsonBytes,
        },
    )
    if err != nil {
        slog.Error("Failed to publish a message", "error", err)
    }

    slog.InfoContext(ctx, "Sent message: "+string(jsonBytes))
}