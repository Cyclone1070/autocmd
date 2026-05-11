package runtimectx

import (
	"context"

	"github.com/Cyclone1070/iav/internal/domain"
)
type ActionWaiter interface {
	Wait(ctx context.Context, callID string) (domain.Action, error)
}

type EventSender interface {
	SendUIUpdate(domain.UIUpdate)
}

type ToolDisplaySink func(callID string, display domain.ToolDisplay)

type contextKey string

const (
	keyActionWaiter    contextKey = "iav/action_waiter"
	keyEventSender     contextKey = "iav/event_sender"
	keyToolDisplaySink contextKey = "iav/tool_display_sink"
)

func WithActionWaiter(ctx context.Context, waiter ActionWaiter) context.Context {
	return context.WithValue(ctx, keyActionWaiter, waiter)
}

func ActionWaiterFrom(ctx context.Context) (ActionWaiter, bool) {
	v, ok := ctx.Value(keyActionWaiter).(ActionWaiter)
	return v, ok
}

func WithEventSender(ctx context.Context, sender EventSender) context.Context {
	return context.WithValue(ctx, keyEventSender, sender)
}

func EventSenderFrom(ctx context.Context) (EventSender, bool) {
	v, ok := ctx.Value(keyEventSender).(EventSender)
	return v, ok
}

func WithToolDisplaySink(ctx context.Context, sink ToolDisplaySink) context.Context {
	return context.WithValue(ctx, keyToolDisplaySink, sink)
}

func ToolDisplaySinkFrom(ctx context.Context) (ToolDisplaySink, bool) {
	v, ok := ctx.Value(keyToolDisplaySink).(ToolDisplaySink)
	return v, ok
}
