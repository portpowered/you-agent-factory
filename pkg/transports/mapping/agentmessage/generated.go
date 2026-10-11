// Package agentmessage maps the public Agent Message representation without
// interpreting authorization, privacy, delivery policy or configured defaults.
package agentmessage

import (
	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Mapper converts values at the generated HTTP boundary. Credentials and
// recipient scope travel separately from the authored send body.
type Mapper struct{}

func (Mapper) SendRequest(input factoryapi.AgentMessageSendRequest) agentmessages.SendRequest {
	result := agentmessages.SendRequest{
		RequestID: input.RequestId, Body: input.Body,
		InReplyTo: value(input.InReplyTo), Delivery: value(input.Delivery),
		IfEnded: value(input.IfEnded), ReplyIfEnded: value(input.ReplyIfEnded),
	}
	if input.BodySecret != nil {
		result.BodySecret = *input.BodySecret
	}
	if input.To != nil {
		result.To = &agentmessages.Address{WorkerSessionID: input.To.WorkerSessionId}
	}
	if input.Correlation != nil {
		result.Correlation = agentmessages.Correlation{
			WorkID: value(input.Correlation.WorkId), FactorySessionID: value(input.Correlation.FactorySessionId),
		}
	}
	if input.ExpiresInSeconds != nil {
		expiry := *input.ExpiresInSeconds
		result.ExpiresInSeconds = &expiry
	}
	return result
}

func (Mapper) Message(input agentmessages.Message) factoryapi.AgentMessage {
	result := factoryapi.AgentMessage{
		MessageId: input.MessageID, ThreadId: input.ThreadID,
		InReplyTo: optional(input.InReplyTo), Body: input.Body,
		BodySha256: input.BodySHA256, BodyRedactionCount: input.BodyRedactionCount,
		Delivery:     factoryapi.AgentMessageDelivery(input.Delivery),
		IfEnded:      factoryapi.AgentMessageIfEnded(input.IfEnded),
		ReplyIfEnded: factoryapi.AgentMessageReplyIfEnded(input.ReplyIfEnded),
		Status:       factoryapi.AgentMessageDeliveryStatus(input.Status), Reason: optional(input.Reason),
		Hop: input.Hop, SentAt: input.SentAt, ExpiresAt: input.ExpiresAt,
		RevivedWorkerSessionId: optional(input.RevivedWorkerSessionID),
		RepliedByMessageId:     optional(input.RepliedByMessageID),
	}
	result.From.Principal = factoryapi.AgentMessageFromPrincipal(input.From.Principal)
	result.From.WorkerSessionId = input.From.WorkerSessionID
	result.From.FactorySessionId = optional(input.From.FactorySessionID)
	result.To.Kind = factoryapi.AgentMessageToKind(input.To.Kind)
	result.To.WorkerSessionId = input.To.WorkerSessionID
	result.To.FactorySessionId = optional(input.To.FactorySessionID)
	result.To.DeliveredWorkerSessionId = optional(input.To.DeliveredWorkerSessionID)
	if input.Correlation != (agentmessages.Correlation{}) {
		result.Correlation = &struct {
			FactorySessionId *string `json:"factorySessionId,omitempty"`
			WorkId           *string `json:"workId,omitempty"`
		}{FactorySessionId: optional(input.Correlation.FactorySessionID), WorkId: optional(input.Correlation.WorkID)}
	}
	return result
}

func (mapper Mapper) Page(input agentmessages.Page) factoryapi.AgentMessageListResponse {
	result := factoryapi.AgentMessageListResponse{
		Messages: make([]factoryapi.AgentMessage, 0, len(input.Messages)), NextToken: optional(input.NextToken),
	}
	for _, message := range input.Messages {
		result.Messages = append(result.Messages, mapper.Message(message))
	}
	return result
}

func value[T ~string](input *T) string {
	if input == nil {
		return ""
	}
	return string(*input)
}

func optional(input string) *string {
	if input == "" {
		return nil
	}
	return &input
}
