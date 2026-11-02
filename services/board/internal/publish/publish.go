// Package publish sends applied board events through pkg/natsx.
package publish

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/drakeRAGE/realtime-whiteboard/pkg/natsx"
)

// Publisher is a JetStream publisher.
type Publisher struct {
	js *natsx.Client
}

// New wraps js. The caller owns the connection.
func New(js *natsx.Client) *Publisher {
	return &Publisher{js: js}
}

// Publish sets Nats-Msg-Id to msgID.
func (p *Publisher) Publish(ctx context.Context, subject, msgID string, msg proto.Message) error {
	if err := p.js.Publish(ctx, subject, msg, msgID); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}
