package clients

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	agentmanager "contestmanager/api/grpc/agentmanager"
)

type AgentManagerClient struct {
	conn   *grpc.ClientConn
	client agentmanager.AgentManagerServiceClient
}

func NewAgentManagerClient(address string) (*AgentManagerClient, error) {
	conn, err := grpc.Dial(address, 
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithTimeout(10*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to agent manager: %w", err)
	}

	return &AgentManagerClient{
		conn:   conn,
		client: agentmanager.NewAgentManagerServiceClient(conn),
	}, nil
}

func (c *AgentManagerClient) CreateAgent(ctx context.Context, contestID, participantID, modelName, contestManagerHost string) (string, error) {
	resp, err := c.client.CreateAgent(ctx, &agentmanager.CreateAgentRequest{
		ContestId:          contestID,
		ParticipantId:      participantID,
		ModelName:         modelName,
		ContestManagerHost: contestManagerHost,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create agent: %w", err)
	}

	log.Printf("Created agent %s for participant %s with model %s", 
		resp.AgentId, participantID, modelName)
	return resp.AgentId, nil
}

func (c *AgentManagerClient) StopAgent(ctx context.Context, agentID, reason string) error {
	resp, err := c.client.StopAgent(ctx, &agentmanager.StopAgentRequest{
		AgentId: agentID,
		Reason:  reason,
	})
	if err != nil {
		return fmt.Errorf("failed to stop agent: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("failed to stop agent: %s", resp.Message)
	}

	log.Printf("Stopped agent %s: %s", agentID, resp.Message)
	return nil
}

func (c *AgentManagerClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
