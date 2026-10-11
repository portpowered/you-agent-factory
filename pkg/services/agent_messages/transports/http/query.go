package http

import (
	"net/url"
	"strconv"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
)

func singleton(query url.Values, key string) (string, error) {
	values, present := query[key]
	if !present {
		return "", nil
	}
	if len(values) != 1 || values[0] == "" {
		return "", agentmessages.ErrBadRequest
	}
	return values[0], nil
}

func listRequest(query url.Values) (agentmessages.ListRequest, bool, error) {
	request := agentmessages.ListRequest{}
	for key, target := range map[string]*string{
		"factorySessionId": &request.FactorySessionID, "toWorkerSessionId": &request.ToWorkerSessionID,
		"fromWorkerSessionId": &request.FromWorkerSessionID, "threadId": &request.ThreadID,
		"correlationWorkId": &request.Correlation.WorkID, "correlationFactorySessionId": &request.Correlation.FactorySessionID,
		"nextToken": &request.NextToken,
	} {
		value, err := singleton(query, key)
		if err != nil {
			return request, false, err
		}
		*target = value
	}
	follow := false
	for key, target := range map[string]*bool{"toMe": &request.ToMe, "markRead": &request.MarkRead, "follow": &follow} {
		value, err := singleton(query, key)
		if err != nil {
			return request, false, err
		}
		if value == "" {
			continue
		}
		if value != "true" && value != "false" {
			return request, false, agentmessages.ErrBadRequest
		}
		*target = value == "true"
	}
	max, err := singleton(query, "maxResults")
	if err != nil {
		return request, false, err
	}
	if max != "" {
		request.MaxResults, err = strconv.Atoi(max)
		if err != nil || request.MaxResults < 1 || request.MaxResults > 500 {
			return request, false, agentmessages.ErrBadRequest
		}
	}
	for _, status := range query["status"] {
		switch agentmessages.Status(status) {
		case agentmessages.Queued, agentmessages.Read, agentmessages.Replied, agentmessages.Expired:
			request.Statuses = append(request.Statuses, agentmessages.Status(status))
		default:
			return request, false, agentmessages.ErrBadRequest
		}
	}
	return request, follow, nil
}
