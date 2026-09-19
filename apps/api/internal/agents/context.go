package agents

import "context"

type ctxKey int

const agentCtxKey ctxKey = 1

func WithAgent(ctx context.Context, agent Agent) context.Context {
	return context.WithValue(ctx, agentCtxKey, agent)
}

func AgentFromContext(ctx context.Context) (Agent, bool) {
	a, ok := ctx.Value(agentCtxKey).(Agent)
	return a, ok
}
