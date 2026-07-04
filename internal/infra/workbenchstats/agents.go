// Agent ranking stats stay in this file so the shared stats repository keeps
// its overview/trend responsibilities small. The query shape mirrors Python's
// assignment-first aggregation while batching message counts for large tables.
package workbenchstats

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"wework-go/internal/workbench"
)

const statsAgentsMessageChunkSize = 500

type statsAgentAccumulator struct {
	name          string
	conversations map[string]struct{}
}

// GetStatsAgents returns Python-compatible assignee workload ranking rows.
func (repository *Repository) GetStatsAgents(ctx context.Context, limit int) ([]workbench.StatsAgentRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench stats database is not configured")
	}
	rows, err := repository.DB.QueryContext(ctx, "SELECT assignee_id, assignee_name, conversation_id FROM conversation_assignments")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agentsByID := map[string]*statsAgentAccumulator{}
	conversationSeen := map[string]struct{}{}
	conversationIDs := make([]string, 0)
	for rows.Next() {
		var assigneeIDValue any
		var assigneeNameValue any
		var conversationIDValue any
		if err := rows.Scan(&assigneeIDValue, &assigneeNameValue, &conversationIDValue); err != nil {
			return nil, err
		}
		assigneeID := stringFromDB(assigneeIDValue)
		conversationID := stringFromDB(conversationIDValue)
		if assigneeID == "" || conversationID == "" {
			continue
		}
		agent := agentsByID[assigneeID]
		if agent == nil {
			agent = &statsAgentAccumulator{
				name:          firstNonEmpty(stringFromDB(assigneeNameValue), assigneeID),
				conversations: map[string]struct{}{},
			}
			agentsByID[assigneeID] = agent
		}
		agent.conversations[conversationID] = struct{}{}
		if _, ok := conversationSeen[conversationID]; !ok {
			conversationSeen[conversationID] = struct{}{}
			conversationIDs = append(conversationIDs, conversationID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(agentsByID) == 0 {
		return []workbench.StatsAgentRecord{}, nil
	}
	outgoingByConversation, err := repository.outgoingCountsByConversation(ctx, conversationIDs)
	if err != nil {
		return nil, err
	}
	records := make([]workbench.StatsAgentRecord, 0, len(agentsByID))
	for assigneeID, agent := range agentsByID {
		messages := 0
		for conversationID := range agent.conversations {
			messages += outgoingByConversation[conversationID]
		}
		records = append(records, workbench.StatsAgentRecord{
			AssigneeID:    assigneeID,
			AssigneeName:  agent.name,
			Conversations: len(agent.conversations),
			Messages:      messages,
		})
	}
	sort.SliceStable(records, func(left int, right int) bool {
		if records[left].Conversations != records[right].Conversations {
			return records[left].Conversations > records[right].Conversations
		}
		if records[left].Messages != records[right].Messages {
			return records[left].Messages > records[right].Messages
		}
		return records[left].AssigneeID < records[right].AssigneeID
	})
	if limit < 1 {
		limit = 1
	}
	if limit > len(records) {
		limit = len(records)
	}
	return records[:limit], nil
}

func (repository *Repository) outgoingCountsByConversation(ctx context.Context, conversationIDs []string) (map[string]int, error) {
	counts := map[string]int{}
	for start := 0; start < len(conversationIDs); start += statsAgentsMessageChunkSize {
		end := start + statsAgentsMessageChunkSize
		if end > len(conversationIDs) {
			end = len(conversationIDs)
		}
		chunk := conversationIDs[start:end]
		query := "SELECT conversation_id, COUNT(*) AS c FROM messages WHERE direction = ? AND conversation_id IN (" + placeholders(len(chunk)) + ") GROUP BY conversation_id"
		args := make([]any, 0, len(chunk)+1)
		args = append(args, "outgoing")
		for _, conversationID := range chunk {
			args = append(args, conversationID)
		}
		rows, err := repository.DB.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var conversationIDValue any
			var countValue any
			if err := rows.Scan(&conversationIDValue, &countValue); err != nil {
				_ = rows.Close()
				return nil, err
			}
			conversationID := stringFromDB(conversationIDValue)
			if conversationID != "" {
				counts[conversationID] = intFromDB(countValue)
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return counts, nil
}

func placeholders(count int) string {
	if count < 1 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
